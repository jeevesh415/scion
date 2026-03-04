// Copyright 2026 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

//go:build !no_sqlite

package hub

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ptone/scion-agent/pkg/api"
	"github.com/ptone/scion-agent/pkg/agent/state"
	"github.com/ptone/scion-agent/pkg/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupNotificationHandlerTest creates a test server with a grove, agent, and
// user subscription with some notifications already stored.
func setupNotificationHandlerTest(t *testing.T) (*Server, store.Store, string) {
	t.Helper()
	srv, s := testServer(t)
	ctx := context.Background()

	grove := &store.Grove{
		ID:   "grove-notif-handler",
		Name: "Notif Handler Grove",
		Slug: "notif-handler-grove",
	}
	require.NoError(t, s.CreateGrove(ctx, grove))

	agent := &store.Agent{
		ID:      "agent-watched",
		Slug:    "watched-agent",
		Name:    "Watched Agent",
		GroveID: grove.ID,
		Phase: string(state.PhaseRunning),
	}
	require.NoError(t, s.CreateAgent(ctx, agent))

	// The dev auth middleware creates a user identity with a deterministic ID.
	// We use "dev-user" as the subscriber ID to match what the middleware produces.
	userID := "dev-user"

	sub := &store.NotificationSubscription{
		ID:              api.NewUUID(),
		AgentID:         agent.ID,
		SubscriberType:  store.SubscriberTypeUser,
		SubscriberID:    userID,
		GroveID:         grove.ID,
		TriggerActivities: []string{"COMPLETED", "WAITING_FOR_INPUT"},
		CreatedAt:       time.Now(),
		CreatedBy:       "test",
	}
	require.NoError(t, s.CreateNotificationSubscription(ctx, sub))

	// Create two notifications: one acknowledged, one not
	notif1 := &store.Notification{
		ID:             api.NewUUID(),
		SubscriptionID: sub.ID,
		AgentID:        agent.ID,
		GroveID:        grove.ID,
		SubscriberType: store.SubscriberTypeUser,
		SubscriberID:   userID,
		Status:         "COMPLETED",
		Message:        "watched-agent has reached a state of COMPLETED",
		Dispatched:     true,
		Acknowledged:   false,
		CreatedAt:      time.Now().Add(-10 * time.Minute),
	}
	require.NoError(t, s.CreateNotification(ctx, notif1))

	notif2 := &store.Notification{
		ID:             api.NewUUID(),
		SubscriptionID: sub.ID,
		AgentID:        agent.ID,
		GroveID:        grove.ID,
		SubscriberType: store.SubscriberTypeUser,
		SubscriberID:   userID,
		Status:         "WAITING_FOR_INPUT",
		Message:        "watched-agent is WAITING_FOR_INPUT",
		Dispatched:     true,
		Acknowledged:   true,
		CreatedAt:      time.Now().Add(-5 * time.Minute),
	}
	require.NoError(t, s.CreateNotification(ctx, notif2))

	return srv, s, notif1.ID
}

func TestHandleNotifications_ListUnacknowledged(t *testing.T) {
	srv, _, _ := setupNotificationHandlerTest(t)

	rec := doRequest(t, srv, http.MethodGet, "/api/v1/notifications", nil)
	assert.Equal(t, http.StatusOK, rec.Code)

	var notifs []store.Notification
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&notifs))

	// Only the unacknowledged notification should be returned
	assert.Len(t, notifs, 1)
	assert.Equal(t, "COMPLETED", notifs[0].Status)
	assert.False(t, notifs[0].Acknowledged)
}

func TestHandleNotifications_ListAll(t *testing.T) {
	srv, _, _ := setupNotificationHandlerTest(t)

	rec := doRequest(t, srv, http.MethodGet, "/api/v1/notifications?acknowledged=true", nil)
	assert.Equal(t, http.StatusOK, rec.Code)

	var notifs []store.Notification
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&notifs))

	// Both notifications should be returned
	assert.Len(t, notifs, 2)
}

func TestHandleNotifications_AcknowledgeSingle(t *testing.T) {
	srv, s, notifID := setupNotificationHandlerTest(t)

	rec := doRequest(t, srv, http.MethodPost, "/api/v1/notifications/"+notifID+"/ack", nil)
	assert.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]string
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	assert.Equal(t, "ok", resp["status"])

	// Verify the notification is now acknowledged
	notifs, err := s.GetNotifications(context.Background(), "user", "dev-user", true)
	require.NoError(t, err)
	for _, n := range notifs {
		if n.ID == notifID {
			assert.True(t, n.Acknowledged)
		}
	}
}

func TestHandleNotifications_AcknowledgeAll(t *testing.T) {
	srv, s, _ := setupNotificationHandlerTest(t)

	rec := doRequest(t, srv, http.MethodPost, "/api/v1/notifications/ack-all", nil)
	assert.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]string
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	assert.Equal(t, "ok", resp["status"])

	// All notifications should now be acknowledged
	notifs, err := s.GetNotifications(context.Background(), "user", "dev-user", true)
	require.NoError(t, err)
	for _, n := range notifs {
		assert.True(t, n.Acknowledged, "notification %s should be acknowledged", n.ID)
	}
}

func TestHandleNotifications_AcknowledgeNotFound(t *testing.T) {
	srv, _, _ := setupNotificationHandlerTest(t)

	rec := doRequest(t, srv, http.MethodPost, "/api/v1/notifications/nonexistent-id/ack", nil)
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestHandleNotifications_RejectAgentToken(t *testing.T) {
	srv, s, _ := setupNotificationHandlerTest(t)
	ctx := context.Background()

	// Create an agent and generate a token for it
	grove := &store.Grove{
		ID:   "grove-agent-auth",
		Name: "Agent Auth Grove",
		Slug: "agent-auth-grove",
	}
	_ = s.CreateGrove(ctx, grove)

	agent := &store.Agent{
		ID:      "agent-auth-test",
		Slug:    "auth-agent",
		Name:    "Auth Agent",
		GroveID: grove.ID,
		Phase: string(state.PhaseRunning),
	}
	require.NoError(t, s.CreateAgent(ctx, agent))

	tokenSvc := srv.GetAgentTokenService()
	require.NotNil(t, tokenSvc)

	agentToken, err := tokenSvc.GenerateAgentToken(agent.ID, grove.ID, []AgentTokenScope{ScopeAgentStatusUpdate})
	require.NoError(t, err)

	// Try to access notifications with an agent token
	req := httptest.NewRequest(http.MethodGet, "/api/v1/notifications", nil)
	req.Header.Set("X-Scion-Agent-Token", agentToken)

	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	assert.Equal(t, http.StatusForbidden, rec.Code)
}

func TestHandleNotifications_MethodNotAllowed(t *testing.T) {
	srv, _, _ := setupNotificationHandlerTest(t)

	// POST to the list endpoint should fail
	rec := doRequest(t, srv, http.MethodPost, "/api/v1/notifications", nil)
	assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
}

func TestHandleNotifications_FilterByAgent(t *testing.T) {
	srv, s, _ := setupNotificationHandlerTest(t)
	ctx := context.Background()

	// The setup already created "agent-watched" with user notifications for "dev-user".
	// Create a second agent that watches "agent-watched", so "agent-watched" is the
	// subscriber (simulating notifications sent TO the watched agent).
	agent2 := &store.Agent{
		ID:      "agent-other",
		Slug:    "other-agent",
		Name:    "Other Agent",
		GroveID: "grove-notif-handler",
		Phase:   string(state.PhaseRunning),
	}
	require.NoError(t, s.CreateAgent(ctx, agent2))

	// Create subscription: agent-watched subscribes to agent-other
	sub2 := &store.NotificationSubscription{
		ID:                api.NewUUID(),
		AgentID:           "agent-other",
		SubscriberType:    store.SubscriberTypeAgent,
		SubscriberID:      "agent-watched",
		GroveID:           "grove-notif-handler",
		TriggerActivities: []string{"COMPLETED"},
		CreatedAt:         time.Now(),
		CreatedBy:         "test",
	}
	require.NoError(t, s.CreateNotificationSubscription(ctx, sub2))

	// Notification sent TO agent-watched (subscriber)
	agentNotif := &store.Notification{
		ID:             api.NewUUID(),
		SubscriptionID: sub2.ID,
		AgentID:        "agent-other",
		GroveID:        "grove-notif-handler",
		SubscriberType: store.SubscriberTypeAgent,
		SubscriberID:   "agent-watched",
		Status:         "COMPLETED",
		Message:        "agent-other completed (to agent-watched)",
		Dispatched:     true,
		Acknowledged:   false,
		CreatedAt:      time.Now(),
	}
	require.NoError(t, s.CreateNotification(ctx, agentNotif))

	// GET with agentId filter
	rec := doRequest(t, srv, http.MethodGet, "/api/v1/notifications?agentId=agent-watched", nil)
	assert.Equal(t, http.StatusOK, rec.Code)

	var resp struct {
		UserNotifications  []store.Notification `json:"userNotifications"`
		AgentNotifications []store.Notification `json:"agentNotifications"`
	}
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))

	// User notifications: 1 unacknowledged for this agent (notif1 from setup)
	assert.Len(t, resp.UserNotifications, 1)
	assert.Equal(t, "COMPLETED", resp.UserNotifications[0].Status)

	// Agent notifications: notifications sent TO agent-watched
	assert.Len(t, resp.AgentNotifications, 1)
	assert.Equal(t, "agent-watched", resp.AgentNotifications[0].SubscriberID)
}

func TestHandleNotifications_FilterByAgent_NoResults(t *testing.T) {
	srv, _, _ := setupNotificationHandlerTest(t)

	// Query for an agent with no notifications
	rec := doRequest(t, srv, http.MethodGet, "/api/v1/notifications?agentId=nonexistent-agent", nil)
	assert.Equal(t, http.StatusOK, rec.Code)

	var resp struct {
		UserNotifications  []store.Notification `json:"userNotifications"`
		AgentNotifications []store.Notification `json:"agentNotifications"`
	}
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	assert.Empty(t, resp.UserNotifications)
	assert.Empty(t, resp.AgentNotifications)
}

func TestHandleNotifications_EmptyList(t *testing.T) {
	srv, _ := testServer(t)

	// No notifications exist for this user
	rec := doRequest(t, srv, http.MethodGet, "/api/v1/notifications", nil)
	assert.Equal(t, http.StatusOK, rec.Code)

	var notifs []store.Notification
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&notifs))
	assert.Empty(t, notifs)
}

// setupGroveWithBroker creates a grove with a registered runtime broker for
// agent creation tests.
func setupGroveWithBroker(t *testing.T, s store.Store, groveID, groveName string) *store.Grove {
	t.Helper()
	ctx := context.Background()

	broker := &store.RuntimeBroker{
		ID:     "broker-" + groveID,
		Name:   "Test Broker",
		Slug:   "test-broker-" + groveID,
		Status: store.BrokerStatusOnline,
	}
	require.NoError(t, s.CreateRuntimeBroker(ctx, broker))

	grove := &store.Grove{
		ID:   groveID,
		Name: groveName,
		Slug: groveID,
	}
	require.NoError(t, s.CreateGrove(ctx, grove))

	provider := &store.GroveProvider{
		GroveID:    grove.ID,
		BrokerID:   broker.ID,
		BrokerName: broker.Name,
		Status:     store.BrokerStatusOnline,
	}
	require.NoError(t, s.AddGroveProvider(ctx, provider))

	return grove
}

func TestCreateGroveAgent_NotifyCreatesSubscription(t *testing.T) {
	srv, s := testServer(t)
	ctx := context.Background()

	grove := setupGroveWithBroker(t, s, "grove-notify-test", "Notify Test Grove")

	// Create an agent via the grove-scoped endpoint with notify=true
	req := CreateAgentRequest{
		Name:   "notify-agent",
		Notify: true,
	}
	rec := doRequest(t, srv, http.MethodPost, "/api/v1/groves/"+grove.ID+"/agents", req)

	// Accept 201 (created) or 202 (env-gather) — either should create the subscription
	assert.True(t, rec.Code == http.StatusCreated || rec.Code == http.StatusAccepted,
		"expected 201 or 202, got %d: %s", rec.Code, rec.Body.String())

	var resp CreateAgentResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	require.NotNil(t, resp.Agent)

	// Verify a notification subscription was created for the user
	subs, err := s.GetNotificationSubscriptions(ctx, resp.Agent.ID)
	require.NoError(t, err)
	require.Len(t, subs, 1, "expected exactly 1 notification subscription for the agent")
	assert.Equal(t, store.SubscriberTypeUser, subs[0].SubscriberType)
	assert.Equal(t, "dev-user", subs[0].SubscriberID)
	assert.Equal(t, grove.ID, subs[0].GroveID)
	assert.Contains(t, subs[0].TriggerActivities, "COMPLETED")
	assert.Contains(t, subs[0].TriggerActivities, "WAITING_FOR_INPUT")
	assert.Contains(t, subs[0].TriggerActivities, "LIMITS_EXCEEDED")
	assert.Contains(t, subs[0].TriggerActivities, "STALLED")
	assert.Contains(t, subs[0].TriggerActivities, "ERROR")
}

func TestCreateGroveAgent_NoNotifyNoSubscription(t *testing.T) {
	srv, s := testServer(t)
	ctx := context.Background()

	grove := setupGroveWithBroker(t, s, "grove-no-notify-test", "No Notify Test Grove")

	// Create an agent without notify
	req := CreateAgentRequest{
		Name: "no-notify-agent",
	}
	rec := doRequest(t, srv, http.MethodPost, "/api/v1/groves/"+grove.ID+"/agents", req)
	assert.True(t, rec.Code == http.StatusCreated || rec.Code == http.StatusAccepted,
		"expected 201 or 202, got %d: %s", rec.Code, rec.Body.String())

	var resp CreateAgentResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	require.NotNil(t, resp.Agent)

	// Verify no subscription was created
	subs, err := s.GetNotificationSubscriptions(ctx, resp.Agent.ID)
	require.NoError(t, err)
	assert.Empty(t, subs, "expected no notification subscriptions when notify is false")
}
