# Hub Permissions System Design

## Status
**Proposed**

## 1. Overview

This document specifies the authorization and access control system for the Scion Hub. The permissions system provides fine-grained access control for resources while maintaining operational simplicity and clear security boundaries.

### Goals

1. **Principal-based access control** - Users and groups as the basis for identity
2. **Hierarchical groups** - Groups can contain users and other groups
3. **Resource-scoped policies** - Policies attached to resources with scope inheritance
4. **CRUD-based actions** - Simple, intuitive action model
5. **Hierarchical policy inheritance** - Hub -> Grove -> Resource with override semantics
6. **Solid debug logging** - Comprehensive audit trail for authorization decisions
7. **Hub-only authorization** - All authz logic in the Hub server

### Non-Goals

- Authentication mechanisms (covered in separate design documents)
- Runtime broker authorization (separate trust model)
- Fine-grained action permissions beyond CRUD (initially)
- Cross-hub federation

---

## 2. Core Concepts

### 2.1 Principals

A **Principal** is an identity that can be granted permissions. There are two types:

#### User

A registered user account with identity and credentials.

```json
{
  "id": "user-uuid",
  "email": "user@example.com",
  "displayName": "Alice Developer",
  "role": "member",
  "status": "active"
}
```

#### Group

A collection of principals (users and/or other groups).

> **Note:** The final permission check is always against a user, as only users take direct actions. When a user attempts a policy-guarded operation, their membership in any groups listed in the policy must be resolved via Ent graph traversals to determine effective access.

```json
{
  "id": "group-uuid",
  "name": "platform-team",
  "slug": "platform-team",
  "description": "Platform engineering team",
  "parentId": "engineering-group-uuid",  // Optional: for group hierarchy
  "members": [
    {"type": "user", "id": "user-uuid-1"},
    {"type": "user", "id": "user-uuid-2"},
    {"type": "group", "id": "devops-group-uuid"}
  ]
}
```

**Group Hierarchy:**
Groups can contain other groups, forming a hierarchy:

```
engineering (group)
├── platform-team (group)
│   ├── alice (user)
│   └── bob (user)
├── frontend-team (group)
│   └── charlie (user)
└── devops (group)
    ├── platform-team (group)  ← nested reference
    └── dave (user)
```


A user gains access to resources through policies attached to those resources (or their containing scopes). When a policy lists a group as a member, all users in that group (directly or transitively) are granted the policy's permissions.

### 2.2 Resources

Resources are the objects that policies protect. The key resources are:

| Resource Type | Description | Containment Scope |
|---------------|-------------|-------------------|
| `hub` | The Hub itself (singleton) | - (root) |
| `grove` | A project/workspace | Hub |
| `agent` | An agent instance | Grove |
| `template` | An agent template | Hub or Grove |
| `user` | A user account | Hub |
| `group` | A user group | Hub |

> **Decision: User-Scoped Templates**
>
> Templates can be personal to a user. Two approaches were considered:
>
> | Approach | Description | Pros | Cons |
> |----------|-------------|------|------|
> | **User as containment scope** | Add `user` as a containment scope alongside hub/grove | Clean hierarchy, user "owns" their templates | Adds complexity to scope resolution, third scope type |
> | **Hub-level with user membership** | Store templates at hub level, add user as policy member | Simpler scope model, reuses existing patterns | Template ownership less explicit, querying user's templates requires policy traversal |
>
> **Decision:** Use hub-level storage with user membership for simplicity. User-scoped containment can be added later if the access patterns become unwieldy.
>
> **Note:** UX considerations for personal template management (discovery, listing, ownership display) will be addressed in later implementation phases.

### 2.3 Resource Scopes (Containment Hierarchy)

Resources exist within a containment hierarchy that determines policy inheritance:

```
Hub (root scope)
├── Users
├── Groups
├── Templates (global scope)
└── Groves
    ├── Templates (grove scope)
    └── Agents
```

**Scope Resolution Path:**
- Agent: `hub -> grove -> agent`
- Template (grove): `hub -> grove -> template`
- Template (global): `hub -> template`
- Grove: `hub -> grove`
- User/Group: `hub -> user/group`

### 2.4 Actions

Actions represent operations that can be performed on resources. The system uses CRUD-based actions:

| Action | Description | Applies To |
|--------|-------------|------------|
| `create` | Create new resource | All |
| `read` | View resource details | All |
| `update` | Modify resource | All |
| `delete` | Remove resource | All |
| `list` | List resources in scope | Container resources |
| `manage` | Administrative operations | Hub, Grove |

**Extended Actions** (resource-specific):

| Resource | Action | Description |
|----------|--------|-------------|
| Agent | `start` | Start the agent |
| Agent | `stop` | Stop the agent |
| Agent | `message` | Send message to agent |
| Agent | `attach` | PTY attachment |
| Grove | `register` | Register grove with Hub |
| Group | `add_member` | Add member to group |
| Group | `remove_member` | Remove member from group |

### 2.5 Policies

A **Policy** defines what actions principals can perform on resources within a scope. Each policy is attached to a single containment scope (hub, grove, or specific resource) and specifies which resource types and actions it governs within that scope.

See [Section 4.3 Policy Data Model](#43-policy) for the detailed schema.

```json
{
  "id": "policy-uuid",
  "name": "platform-team-grove-admin",
  "description": "Full access to grove resources",

  // Scope attachment (single scope per policy)
  "scopeType": "grove",
  "scopeId": "grove-uuid",

  // What resources this policy covers within the scope
  "resourceType": "agent",        // Or "*" for all types
  "resourceId": null,             // Optional: specific resource

  "actions": ["create", "read", "update", "delete", "manage"],

  "effect": "allow",              // "allow" or "deny"

  "conditions": {                 // Optional conditions
    "labels": {"environment": "production"}
  }
}
```

Principals are bound to policies via `PolicyBinding` records (see [Section 4.4](#44-policybinding)).

---

## 3. Policy Resolution

### 3.1 Resolution Order

Policy resolution follows the containment hierarchy from **higher to lower levels**, with **lower levels overriding higher levels**:

```
1. Hub-level policies (default, lowest priority)
2. Grove-level policies
3. Resource-specific policies (highest priority)
```

This means:
- A policy at the grove level overrides a hub-level policy
- A policy attached to a specific agent overrides grove-level policies

### 3.2 Override Semantics

**Important:** This design uses an **override model**, not a **least-privilege model**.

| Approach | Behavior | Example |
|----------|----------|---------|
| **Override (chosen)** | Lower-level policies replace higher-level | Hub: deny all -> Grove: allow read -> Agent has read |
| **Least-privilege** | Most restrictive wins | Hub: deny all -> Grove: allow read -> Agent denied |

**Rationale for Override:**
1. **Delegation** - Grove owners can grant access without Hub admin intervention
2. **Autonomy** - Teams can manage their own grove permissions
3. **Simplicity** - Easier to reason about: "what's set here wins"

**Trade-offs:**
- Risk: Lower-level admins can grant access that Hub admins might not want
- Mitigation: Use `deny` policies at higher levels that cannot be overridden (see "Hard Deny" below)

### 3.3 Resolution Algorithm

> **Note:** This is an illustrative algorithm. The actual implementation may differ based on Ent query patterns and performance optimizations discovered during development.

```go
func resolveAccess(ctx context.Context, principal Principal, resource Resource, action Action) Decision {
    log := authzLogger(ctx)

    // Step 1: Expand principal's effective groups (flatten hierarchy)
    effectiveGroups := expandGroups(ctx, principal)
    allPrincipals := append([]Principal{principal}, effectiveGroups...)

    log.Debug("resolving access",
        "principal", principal.ID,
        "resource", resource.ID,
        "action", action,
        "effectiveGroups", len(effectiveGroups))

    // Step 2: Collect policies at each scope level
    scopes := getResourceScopes(resource) // e.g., [hub, grove, agent]

    var resolvedDecision *Decision = nil

    for _, scope := range scopes {
        policies := getPoliciesForScope(ctx, scope, allPrincipals)

        for _, policy := range policies {
            if matchesResource(policy, resource) && matchesAction(policy, action) {
                decision := evaluatePolicy(policy, ctx)

                log.Debug("policy matched",
                    "policy", policy.ID,
                    "scope", scope,
                    "effect", policy.Effect,
                    "decision", decision)

                // Future: hard_deny check would go here (see Section 3.4)

                // Lower level overrides higher level
                resolvedDecision = &Decision{
                    Allowed: policy.Effect == "allow",
                    Reason:  policy.Effect,
                    Policy:  policy,
                    Scope:   scope,
                }
            }
        }
    }

    // Step 3: Apply default (deny if no policy matched)
    if resolvedDecision == nil {
        log.Debug("no matching policy, applying default deny",
            "principal", principal.ID,
            "resource", resource.ID,
            "action", action)
        return Decision{Allowed: false, Reason: "no_matching_policy"}
    }

    log.Info("access decision",
        "allowed", resolvedDecision.Allowed,
        "principal", principal.ID,
        "resource", resource.ID,
        "action", action,
        "policy", resolvedDecision.Policy.ID,
        "scope", resolvedDecision.Scope)

    return *resolvedDecision
}
```

### 3.4 Policy Effects

| Effect | Behavior | Override-able |
|--------|----------|---------------|
| `allow` | Grants access | Yes |
| `deny` | Denies access | Yes (by lower-level allow) |

> **Future Enhancement: Hard Deny**
>
> A `hard_deny` effect could provide Hub admins a mechanism to enforce restrictions that grove owners cannot override. This is documented in [Section 12.1](#121-policy-resolution-override-vs-least-privilege) as a potential future refinement. For initial implementation, the override model with standard `deny` is sufficient.

---

## 4. Data Models

### 4.1 Group

> **Initial Proposal:** This schema may be refined based on [Ent traversal best practices](https://entgo.io/docs/traversals/) during implementation.

```go
// Group represents a user group that can contain users and other groups.
type Group struct {
    // Identity
    ID          string `json:"id"`          // UUID primary key
    Name        string `json:"name"`        // Human-friendly name
    Slug        string `json:"slug"`        // URL-safe identifier
    Description string `json:"description,omitempty"`

    // Hierarchy
    ParentID string `json:"parentId,omitempty"` // Parent group (for hierarchy)

    // Metadata
    Labels      map[string]string `json:"labels,omitempty"`
    Annotations map[string]string `json:"annotations,omitempty"`

    // Timestamps
    Created time.Time `json:"created"`
    Updated time.Time `json:"updated"`

    // Ownership
    CreatedBy string `json:"createdBy,omitempty"`
    OwnerID   string `json:"ownerId,omitempty"`
}
```

### 4.2 GroupMember

```go
// GroupMember represents membership in a group.
type GroupMember struct {
    GroupID     string    `json:"groupId"`     // FK to Group.ID
    MemberType  string    `json:"memberType"`  // "user" or "group"
    MemberID    string    `json:"memberId"`    // FK to User.ID or Group.ID
    Role        string    `json:"role"`        // "member", "admin", "owner"
    AddedAt     time.Time `json:"addedAt"`
    AddedBy     string    `json:"addedBy,omitempty"`
}

// MemberType constants
const (
    MemberTypeUser  = "user"
    MemberTypeGroup = "group"
)
```

### 4.3 Policy

```go
// Policy defines access control rules.
type Policy struct {
    // Identity
    ID          string `json:"id"`   // UUID primary key
    Name        string `json:"name"` // Human-friendly name
    Description string `json:"description,omitempty"`

    // Scope attachment
    ScopeType string `json:"scopeType"` // "hub", "grove", "resource"
    ScopeID   string `json:"scopeId"`   // ID of the scope (grove ID, resource ID, or empty for hub)

    // What the policy applies to
    ResourceType string   `json:"resourceType"` // "agent", "grove", "template", etc. or "*" for all
    ResourceID   string   `json:"resourceId,omitempty"` // Specific resource (optional)
    Actions      []string `json:"actions"`      // ["create", "read", "update", "delete", ...]

    // Effect
    Effect string `json:"effect"` // "allow", "deny" (future: "hard_deny")

    // Conditions (optional)
    Conditions *PolicyConditions `json:"conditions,omitempty"`

    // Priority for ordering within same scope (higher = evaluated later, can override)
    Priority int `json:"priority"`

    // Metadata
    Labels      map[string]string `json:"labels,omitempty"`
    Annotations map[string]string `json:"annotations,omitempty"`

    // Timestamps
    Created time.Time `json:"created"`
    Updated time.Time `json:"updated"`

    // Ownership
    CreatedBy string `json:"createdBy,omitempty"`
}
```

### 4.4 PolicyBinding

Policies are attached to resources (or scopes) via the `Policy.ScopeType` and `Policy.ScopeID` fields (see [Section 4.3](#43-policy)). The `PolicyBinding` record links principals (users or groups) to a policy, granting those principals the permissions defined by the policy.

This model supports many principals having access to the same resource through a single policy.

```go
// PolicyBinding links principals to a policy.
// The policy itself is attached to a resource/scope via its ScopeType and ScopeID fields.
type PolicyBinding struct {
    PolicyID      string `json:"policyId"`      // FK to Policy.ID
    PrincipalType string `json:"principalType"` // "user" or "group"
    PrincipalID   string `json:"principalId"`   // FK to User.ID or Group.ID
}
```

**Lookup Pattern:** To find all principals with access to a resource:
1. Find policies where `ScopeID` matches the resource (or its containing scope)
2. Query `PolicyBinding` for all principals bound to those policies
3. Expand group memberships to get individual users

### 4.5 PolicyConditions

```go
// PolicyConditions defines optional conditions for policy matching.
type PolicyConditions struct {
    // Label matching
    Labels map[string]string `json:"labels,omitempty"`

    // Time-based conditions
    ValidFrom  *time.Time `json:"validFrom,omitempty"`
    ValidUntil *time.Time `json:"validUntil,omitempty"`

    // IP-based conditions (for future use)
    SourceIPs []string `json:"sourceIps,omitempty"`
}
```

---

## 5. Database Schema (Ent)

The implementation uses [entgo.io](https://entgo.io/) for the persistence layer, particularly for managing the user-group hierarchy with its graph-like relationships.

### 5.1 Ent Schema: Group

```go
// Group holds the schema definition for the Group entity.
type Group struct {
    ent.Schema
}

// Fields of the Group.
func (Group) Fields() []ent.Field {
    return []ent.Field{
        field.UUID("id", uuid.UUID{}).Default(uuid.New),
        field.String("name").NotEmpty(),
        field.String("slug").NotEmpty().Unique(),
        field.String("description").Optional(),
        field.JSON("labels", map[string]string{}).Optional(),
        field.JSON("annotations", map[string]string{}).Optional(),
        field.Time("created").Default(time.Now),
        field.Time("updated").Default(time.Now).UpdateDefault(time.Now),
        field.String("created_by").Optional(),
        field.String("owner_id").Optional(),
    }
}

// Edges of the Group.
func (Group) Edges() []ent.Edge {
    return []ent.Edge{
        // Parent group (hierarchical relationship)
        edge.To("parent", Group.Type).Unique(),
        edge.From("children", Group.Type).Ref("parent"),

        // Member users
        edge.To("users", User.Type),

        // Member groups (groups within this group)
        edge.To("member_groups", Group.Type),
        edge.From("parent_groups", Group.Type).Ref("member_groups"),

        // Policies attached to this group
        edge.From("policies", Policy.Type).Ref("principals_groups"),
    }
}
```

### 5.2 Ent Schema: Policy

```go
// Policy holds the schema definition for the Policy entity.
type Policy struct {
    ent.Schema
}

// Fields of the Policy.
func (Policy) Fields() []ent.Field {
    return []ent.Field{
        field.UUID("id", uuid.UUID{}).Default(uuid.New),
        field.String("name").NotEmpty(),
        field.String("description").Optional(),

        field.Enum("scope_type").Values("hub", "grove", "resource"),
        field.String("scope_id").Optional(), // Grove ID or resource ID

        field.String("resource_type").Default("*"),
        field.String("resource_id").Optional(),
        field.JSON("actions", []string{}),

        field.Enum("effect").Values("allow", "deny"), // Future: add "hard_deny"

        field.JSON("conditions", &PolicyConditions{}).Optional(),
        field.Int("priority").Default(0),

        field.JSON("labels", map[string]string{}).Optional(),
        field.JSON("annotations", map[string]string{}).Optional(),
        field.Time("created").Default(time.Now),
        field.Time("updated").Default(time.Now).UpdateDefault(time.Now),
        field.String("created_by").Optional(),
    }
}

// Edges of the Policy.
func (Policy) Edges() []ent.Edge {
    return []ent.Edge{
        // Principal bindings
        edge.To("principals_users", User.Type),
        edge.To("principals_groups", Group.Type),

        // Scope attachment (for grove-level policies)
        edge.To("grove", Grove.Type).Unique(),
    }
}
```

### 5.3 Ent Schema Updates: User

Add group membership edge to existing User schema:

```go
// Edges of the User (additions).
func (User) Edges() []ent.Edge {
    return []ent.Edge{
        // Group memberships
        edge.From("groups", Group.Type).Ref("users"),

        // Policies attached to this user
        edge.From("policies", Policy.Type).Ref("principals_users"),
    }
}
```

---

## 6. API Endpoints

### 6.1 Group Endpoints

#### List Groups
```
GET /api/v1/groups
```

**Query Parameters:**
| Parameter | Type | Description |
|-----------|------|-------------|
| `parentId` | string | Filter by parent group |
| `memberId` | string | Filter groups containing this member |
| `limit` | int | Maximum results |
| `cursor` | string | Pagination cursor |

#### Get Group
```
GET /api/v1/groups/{groupId}
```

#### Create Group
```
POST /api/v1/groups
```

**Request Body:**
```json
{
  "name": "Platform Team",
  "slug": "platform-team",
  "description": "Platform engineering team",
  "parentId": "engineering-uuid"
}
```

#### Update Group
```
PATCH /api/v1/groups/{groupId}
```

#### Delete Group
```
DELETE /api/v1/groups/{groupId}
```

#### List Group Members
```
GET /api/v1/groups/{groupId}/members
```

#### Add Group Member
```
POST /api/v1/groups/{groupId}/members
```

**Request Body:**
```json
{
  "memberType": "user",
  "memberId": "user-uuid",
  "role": "member"
}
```

#### Remove Group Member
```
DELETE /api/v1/groups/{groupId}/members/{memberType}/{memberId}
```

### 6.2 Policy Endpoints

#### List Policies
```
GET /api/v1/policies
```

**Query Parameters:**
| Parameter | Type | Description |
|-----------|------|-------------|
| `scopeType` | string | Filter by scope type (hub, grove, resource) |
| `scopeId` | string | Filter by scope ID |
| `resourceType` | string | Filter by resource type |
| `principalId` | string | Filter policies affecting this principal |

#### Get Policy
```
GET /api/v1/policies/{policyId}
```

#### Create Policy
```
POST /api/v1/policies
```

**Request Body:**
```json
{
  "name": "Grove Admin Policy",
  "description": "Full access to grove resources",
  "scopeType": "grove",
  "scopeId": "grove-uuid",
  "resourceType": "*",
  "actions": ["create", "read", "update", "delete", "manage"],
  "effect": "allow",
  "principals": [
    {"type": "group", "id": "platform-team-uuid"}
  ]
}
```

#### Update Policy
```
PATCH /api/v1/policies/{policyId}
```

#### Delete Policy
```
DELETE /api/v1/policies/{policyId}
```

#### Evaluate Access (Debug/Test)
```
POST /api/v1/policies/evaluate
```

**Request Body:**
```json
{
  "principalType": "user",
  "principalId": "user-uuid",
  "resourceType": "agent",
  "resourceId": "agent-uuid",
  "action": "delete"
}
```

**Response:**
```json
{
  "allowed": true,
  "reason": "allow",
  "matchedPolicy": {
    "id": "policy-uuid",
    "name": "Grove Admin Policy",
    "scopeType": "grove",
    "effect": "allow"
  },
  "evaluationPath": [
    {"scope": "hub", "policies": 2, "matched": 0},
    {"scope": "grove", "policies": 1, "matched": 1}
  ],
  "effectiveGroups": ["platform-team-uuid", "engineering-uuid"]
}
```

---

## 7. Resource-Scoped Policy Attachment

### 7.1 Grove-Level Policies

Groves can have policies attached that apply to all resources within:

```
GET /api/v1/groves/{groveId}/policies
POST /api/v1/groves/{groveId}/policies
DELETE /api/v1/groves/{groveId}/policies/{policyId}
```

### 7.2 Hub-Level Policies

Hub-level policies apply globally:

```
GET /api/v1/hub/policies
POST /api/v1/hub/policies
DELETE /api/v1/hub/policies/{policyId}
```

### 7.3 Resource-Specific Policies

Policies can be attached to specific resources:

```
GET /api/v1/agents/{agentId}/policies
POST /api/v1/agents/{agentId}/policies
DELETE /api/v1/agents/{agentId}/policies/{policyId}
```

---

## 8. Built-in Roles and Policies

### 8.1 System Roles

The system includes predefined roles for common access patterns:

| Role | Description | Permissions |
|------|-------------|-------------|
| `hub:admin` | Full Hub administration | All actions on all resources |
| `hub:member` | Standard Hub user | Create groves, manage own resources |
| `hub:viewer` | Read-only access | Read all visible resources |
| `grove:admin` | Grove administration | All actions within grove |
| `grove:member` | Grove member | Create/manage agents in grove |
| `grove:viewer` | Grove read-only | Read grove resources |

### 8.2 Default Policies

On Hub initialization, create default policies:

```go
var defaultPolicies = []Policy{
    {
        Name:         "hub-admin-full-access",
        Description:  "Hub administrators have full access",
        ScopeType:    "hub",
        ResourceType: "*",
        Actions:      []string{"*"},
        Effect:       "allow",
        // Bound to group: hub-admins
    },
    {
        Name:         "hub-member-create-groves",
        Description:  "Hub members can create groves",
        ScopeType:    "hub",
        ResourceType: "grove",
        Actions:      []string{"create"},
        Effect:       "allow",
        // Bound to group: hub-members
    },
    {
        Name:         "resource-owner-full-access",
        Description:  "Resource owners have full access to their resources",
        ScopeType:    "hub",
        ResourceType: "*",
        Actions:      []string{"*"},
        Effect:       "allow",
        Conditions: &PolicyConditions{
            // Special condition: principal.id == resource.ownerId
            // Implemented in code, not as label match
        },
    },
}
```

---

## 9. Authorization Middleware

### 9.1 HTTP Middleware

```go
// AuthzMiddleware enforces authorization on API requests.
func AuthzMiddleware(authz *AuthzService) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            ctx := r.Context()
            log := authzLogger(ctx)

            // Get authenticated user from context
            user := auth.UserFromContext(ctx)
            if user == nil {
                log.Warn("no user in context")
                http.Error(w, "unauthorized", http.StatusUnauthorized)
                return
            }

            // Extract resource and action from request
            resource, action := extractResourceAndAction(r)

            log.Debug("authorization check",
                "user", user.ID,
                "method", r.Method,
                "path", r.URL.Path,
                "resource", resource,
                "action", action)

            // Check authorization
            decision := authz.CheckAccess(ctx, user, resource, action)

            if !decision.Allowed {
                log.Warn("access denied",
                    "user", user.ID,
                    "resource", resource,
                    "action", action,
                    "reason", decision.Reason)

                writeJSON(w, http.StatusForbidden, map[string]interface{}{
                    "error": map[string]interface{}{
                        "code":    "forbidden",
                        "message": "You don't have permission to perform this action",
                        "details": map[string]string{
                            "resource": resource.Type + "/" + resource.ID,
                            "action":   string(action),
                            "reason":   decision.Reason,
                        },
                    },
                })
                return
            }

            log.Debug("access granted",
                "user", user.ID,
                "resource", resource,
                "action", action,
                "policy", decision.Policy.ID)

            next.ServeHTTP(w, r)
        })
    }
}
```

### 9.2 Resource and Action Extraction

```go
// extractResourceAndAction determines the resource and action from an HTTP request.
func extractResourceAndAction(r *http.Request) (Resource, Action) {
    path := r.URL.Path
    method := r.Method

    // Map HTTP methods to actions
    methodToAction := map[string]Action{
        "GET":    ActionRead,
        "POST":   ActionCreate,
        "PUT":    ActionUpdate,
        "PATCH":  ActionUpdate,
        "DELETE": ActionDelete,
    }

    action := methodToAction[method]

    // Parse path to determine resource type and ID
    // e.g., /api/v1/groves/grove-123/agents/agent-456

    parts := strings.Split(strings.Trim(path, "/"), "/")

    var resource Resource

    // Traverse path parts to build resource context
    for i := 0; i < len(parts); i++ {
        switch parts[i] {
        case "groves":
            if i+1 < len(parts) && parts[i+1] != "register" {
                resource = Resource{Type: "grove", ID: parts[i+1]}
                i++
            }
        case "agents":
            if i+1 < len(parts) {
                resource = Resource{Type: "agent", ID: parts[i+1], ParentType: "grove", ParentID: resource.ID}
                i++
            } else {
                // Listing agents
                action = ActionList
                resource.Type = "agent"
            }
        // ... handle other resource types
        }
    }

    // Handle action overrides from path
    if len(parts) > 0 {
        lastPart := parts[len(parts)-1]
        switch lastPart {
        case "start":
            action = ActionStart
        case "stop":
            action = ActionStop
        case "message":
            action = ActionMessage
        }
    }

    return resource, action
}
```

---

## 10. Debug Logging

### 10.1 Log Levels

| Level | Usage |
|-------|-------|
| DEBUG | Policy evaluation details, group expansion |
| INFO | Authorization decisions (allow/deny) |
| WARN | Denied access, potential security issues |
| ERROR | Authorization system failures |

### 10.2 Structured Log Fields

All authorization logs include:

```go
type AuthzLogFields struct {
    // Request context
    RequestID   string `json:"requestId"`
    UserID      string `json:"userId"`
    UserEmail   string `json:"userEmail,omitempty"`

    // Resource context
    ResourceType string `json:"resourceType"`
    ResourceID   string `json:"resourceId"`
    Action       string `json:"action"`

    // Decision
    Allowed bool   `json:"allowed"`
    Reason  string `json:"reason"`

    // Policy context
    PolicyID    string `json:"policyId,omitempty"`
    PolicyName  string `json:"policyName,omitempty"`
    PolicyScope string `json:"policyScope,omitempty"`

    // Evaluation details
    EffectiveGroups []string `json:"effectiveGroups,omitempty"`
    EvaluatedPolicies int    `json:"evaluatedPolicies,omitempty"`

    // Timing
    EvaluationMs float64 `json:"evaluationMs,omitempty"`
}
```

### 10.3 Example Log Output

```json
{
  "level": "info",
  "ts": "2025-01-24T10:30:00.123Z",
  "msg": "access decision",
  "requestId": "req-abc123",
  "userId": "user-456",
  "userEmail": "alice@example.com",
  "resourceType": "agent",
  "resourceId": "agent-789",
  "action": "delete",
  "allowed": true,
  "reason": "allow",
  "policyId": "policy-xyz",
  "policyName": "Grove Admin Policy",
  "policyScope": "grove",
  "effectiveGroups": ["platform-team", "engineering"],
  "evaluatedPolicies": 3,
  "evaluationMs": 2.5
}
```

---

## 11. Security Considerations

### 11.1 Cycle Detection in Group Hierarchy

Group membership can form cycles (A contains B contains A). The system must:

1. Detect cycles during group membership addition
2. Prevent cycles from being created
3. Handle existing cycles gracefully in resolution

```go
func (s *GroupService) AddMember(ctx context.Context, groupID, memberID, memberType string) error {
    if memberType == MemberTypeGroup {
        // Check for cycles
        if wouldCreateCycle(ctx, groupID, memberID) {
            return ErrCycleDetected
        }
    }
    // ... add member
}
```

### 11.2 Policy Evaluation Performance

Group hierarchy expansion can be expensive. Mitigations:

1. **Cache effective groups** - Per-request cache of expanded groups
2. **Limit hierarchy depth** - Maximum 10 levels of group nesting
3. **Eager evaluation** - Pre-compute effective groups on login

### 11.3 Audit Trail

All policy changes are logged:

```go
type PolicyAuditEvent struct {
    EventType   string    `json:"eventType"`   // created, updated, deleted
    PolicyID    string    `json:"policyId"`
    PolicyName  string    `json:"policyName"`
    ChangedBy   string    `json:"changedBy"`
    ChangedAt   time.Time `json:"changedAt"`
    OldValue    *Policy   `json:"oldValue,omitempty"`
    NewValue    *Policy   `json:"newValue,omitempty"`
}
```

---

## 12. Alternative Approaches Considered

### 12.1 Policy Resolution: Override vs Least-Privilege

| Aspect | Override Model (Chosen) | Least-Privilege Model |
|--------|-------------------------|------------------------|
| Philosophy | Lower levels can expand access | Most restrictive wins |
| Delegation | Natural: grove owners can grant | Requires hub admin involvement |
| Risk | Lower admins might over-grant | Lower admins can only restrict |
| Complexity | Simpler mental model | Requires understanding all levels |
| Revocation | Must delete/modify lower policy | Add deny at any level |

**Decision:** Override model chosen for delegation flexibility.

> **Future Improvement: Hard Deny**
>
> A `hard_deny` effect could be added to allow Hub admins to enforce restrictions that cannot be overridden by lower-level policies. This would provide an escape hatch for critical restrictions (e.g., preventing deletion of production agents) while maintaining the delegation benefits of the override model.
>
> Implementation would add a third effect type that short-circuits the resolution algorithm before override logic applies.

### 12.2 Group Hierarchy: Ent vs Custom Implementation

| Aspect | Ent (Chosen) | Custom Implementation |
|--------|--------------|----------------------|
| Development speed | Fast: built-in graph traversal | Slow: manual SQL |
| Type safety | Strong: generated code | Weak: raw queries |
| Cycle handling | Built-in tools | Manual implementation |
| Performance | Good with proper indexes | Potentially better with optimization |
| Complexity | Learning curve | Full control |

**Decision:** Ent chosen for development speed and type safety, especially for complex group hierarchies.

### 12.3 Policy Attachment: Inline vs Referenced

| Aspect | Referenced Policies (Chosen) | Inline Policies |
|--------|------------------------------|-----------------|
| Reusability | High: same policy, multiple bindings | Low: duplicate policies |
| Management | Centralized policy management | Scattered across resources |
| Audit | Clear policy ownership | Harder to track |
| Flexibility | Update once, affects all bindings | Must update each inline |

**Decision:** Referenced policies with bindings for reusability and centralized management.

---

## 13. Implementation Phases

### Phase 1: Foundation
- [ ] Add Group and GroupMember schemas (Ent)
- [ ] Add Policy and PolicyBinding schemas (Ent)
- [ ] Implement group CRUD API endpoints
- [ ] Implement group membership API endpoints
- [ ] Add cycle detection for group hierarchy

### Phase 2: Policy Engine
- [ ] Implement policy CRUD API endpoints
- [ ] Implement policy resolution algorithm
- [ ] Implement group expansion (transitive membership)
- [ ] Add authorization middleware
- [ ] Integrate with existing handlers

### Phase 3: Built-in Policies
- [ ] Create default groups (hub-admins, hub-members)
- [ ] Create default policies
- [ ] Implement owner-based access check
- [ ] Add hub initialization with default policies

### Phase 4: Debug & Audit
- [ ] Implement structured authorization logging
- [ ] Add policy evaluation endpoint (debug)
- [ ] Add audit trail for policy changes
- [ ] Add authorization metrics

### Phase 5: UI Integration
- [ ] Add permissions management to web dashboard
- [ ] Add group management UI
- [ ] Add policy management UI
- [ ] Add access visualization

---

## 14. References

- **Hosted Architecture:** `hosted-architecture.md`
- **Hub API Specification:** `hub-api.md`
- **Development Authentication:** `dev-auth.md`
- **Ent Documentation:** https://entgo.io/
