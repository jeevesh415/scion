package runtime

import (
	"context"
	"fmt"
)

type MockRuntime struct {
	Agents map[string]AgentInfo
}

func NewMockRuntime() *MockRuntime {
	return &MockRuntime{
		Agents: make(map[string]AgentInfo),
	}
}

func (m *MockRuntime) Run(ctx context.Context, config RunConfig) (string, error) {
	id := fmt.Sprintf("id-%s", config.Name)
	m.Agents[id] = AgentInfo{
		ID:     id,
		Name:   config.Name,
		Status: "Running",
		Image:  config.Image,
	}
	return id, nil
}

func (m *MockRuntime) Stop(ctx context.Context, id string) error {
	if _, ok := m.Agents[id]; !ok {
		return fmt.Errorf("agent not found")
	}
	delete(m.Agents, id)
	return nil
}

func (m *MockRuntime) List(ctx context.Context, labelFilter map[string]string) ([]AgentInfo, error) {
	var list []AgentInfo
	for _, a := range m.Agents {
		list = append(list, a)
	}
	return list, nil
}

func (m *MockRuntime) GetLogs(ctx context.Context, id string) (string, error) {
	return "mock logs", nil
}

func (m *MockRuntime) Attach(ctx context.Context, id string) error {
	fmt.Printf("Mock: Attaching to %s\n", id)
	return nil
}

func (m *MockRuntime) ImageExists(ctx context.Context, image string) (bool, error) {
	return true, nil
}
