package httpapi

import (
	"github.com/graphql-go/graphql"

	"github.com/ArsinShaabani/AIOS/internal/domain"
	"github.com/ArsinShaabani/AIOS/internal/ports"
)

type gqlTypes struct {
	user     *graphql.Object
	session  *graphql.Object
	agent    *graphql.Object
	memory   *graphql.Object
	device   *graphql.Object
	overview *graphql.Object
	provider *graphql.Object
	reply    *graphql.Object
	usage    *graphql.Object
}

func str(name string) *graphql.Field {
	return &graphql.Field{Type: graphql.String, Resolve: fieldName(name)}
}

func fieldName(name string) graphql.FieldResolveFn {
	return func(p graphql.ResolveParams) (any, error) {
		return pick(p.Source, name), nil
	}
}

// pick extracts a snake_case or camelCase field from known domain structs.
func pick(src any, name string) any {
	switch v := src.(type) {
	case domain.User:
		switch name {
		case "id":
			return v.ID
		case "email":
			return v.Email
		case "displayName":
			return v.DisplayName
		case "role":
			return string(v.Role)
		case "status":
			return v.Status
		case "timezone":
			return v.Timezone
		}
	case domain.Session:
		switch name {
		case "id":
			return v.ID
		case "title":
			return v.Title
		case "agentId":
			return v.AgentID
		case "messageCount":
			return v.MessageCount
		case "updatedAt":
			return v.UpdatedAt.Format("2006-01-02T15:04:05Z")
		}
	case domain.Agent:
		switch name {
		case "id":
			return v.ID
		case "name":
			return v.Name
		case "description":
			return v.Description
		case "enabled":
			return v.Enabled
		case "tools":
			return v.Tools
		}
	case domain.Memory:
		switch name {
		case "id":
			return v.ID
		case "kind":
			return string(v.Kind)
		case "content":
			return v.Content
		case "importance":
			return v.Importance
		case "createdAt":
			return v.CreatedAt.Format("2006-01-02T15:04:05Z")
		}
	case domain.Device:
		switch name {
		case "id":
			return v.ID
		case "name":
			return v.Name
		case "kind":
			return string(v.Kind)
		case "room":
			return v.Room
		case "status":
			return v.Status
		}
	case ports.ProviderSummary:
		switch name {
		case "name":
			return v.Name
		case "kind":
			return v.Kind
		case "healthy":
			return v.Healthy
		case "models":
			return v.Models
		}
	case map[string]any:
		return v[name]
	}
	return nil
}

func (a *API) gqlTypes() *gqlTypes {
	t := &gqlTypes{}
	t.user = graphql.NewObject(graphql.ObjectConfig{Name: "User", Fields: graphql.Fields{
		"id": str("id"), "email": str("email"), "displayName": str("displayName"),
		"role": str("role"), "status": str("status"), "timezone": str("timezone"),
	}})
	t.session = graphql.NewObject(graphql.ObjectConfig{Name: "Session", Fields: graphql.Fields{
		"id": str("id"), "title": str("title"), "agentId": str("agentId"),
		"messageCount": &graphql.Field{Type: graphql.Int, Resolve: fieldName("messageCount")},
		"updatedAt":    str("updatedAt"),
	}})
	t.agent = graphql.NewObject(graphql.ObjectConfig{Name: "Agent", Fields: graphql.Fields{
		"id": str("id"), "name": str("name"), "description": str("description"),
		"enabled": &graphql.Field{Type: graphql.Boolean, Resolve: fieldName("enabled")},
		"tools":   &graphql.Field{Type: graphql.NewList(graphql.String), Resolve: fieldName("tools")},
	}})
	t.memory = graphql.NewObject(graphql.ObjectConfig{Name: "Memory", Fields: graphql.Fields{
		"id": str("id"), "kind": str("kind"), "content": str("content"),
		"importance": &graphql.Field{Type: graphql.Float, Resolve: fieldName("importance")},
		"createdAt":  str("createdAt"),
	}})
	t.device = graphql.NewObject(graphql.ObjectConfig{Name: "Device", Fields: graphql.Fields{
		"id": str("id"), "name": str("name"), "kind": str("kind"), "room": str("room"), "status": str("status"),
	}})
	t.overview = graphql.NewObject(graphql.ObjectConfig{Name: "Overview", Fields: graphql.Fields{
		"users": &graphql.Field{Type: graphql.Int}, "sessions": &graphql.Field{Type: graphql.Int},
		"messages": &graphql.Field{Type: graphql.Int}, "devices": &graphql.Field{Type: graphql.Int},
		"memories": &graphql.Field{Type: graphql.Int},
	}})
	t.provider = graphql.NewObject(graphql.ObjectConfig{Name: "Provider", Fields: graphql.Fields{
		"name": str("name"), "kind": str("kind"),
		"healthy": &graphql.Field{Type: graphql.Boolean, Resolve: fieldName("healthy")},
		"models":  &graphql.Field{Type: graphql.NewList(graphql.String), Resolve: fieldName("models")},
	}})
	t.reply = graphql.NewObject(graphql.ObjectConfig{Name: "Reply", Fields: graphql.Fields{
		"sessionId": str("sessionId"), "content": str("content"), "agentId": str("agentId"),
		"provider": str("provider"), "model": str("model"),
	}})
	return t
}

func (a *API) schema() (graphql.Schema, error) {
	a.gqlOnce.Do(func() {
		t := a.gqlTypes()
		a.gqlSchema, a.gqlErr = graphql.NewSchema(graphql.SchemaConfig{
			Query:    graphql.NewObject(graphql.ObjectConfig{Name: "Query", Fields: a.queryFields(t)}),
			Mutation: graphql.NewObject(graphql.ObjectConfig{Name: "Mutation", Fields: a.mutationFields(t)}),
		})
	})
	return a.gqlSchema, a.gqlErr
}
