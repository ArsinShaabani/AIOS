package httpapi

import (
	"github.com/graphql-go/graphql"

	"github.com/ArsinShaabani/AIOS/internal/brain"
	"github.com/ArsinShaabani/AIOS/internal/domain"
	"github.com/ArsinShaabani/AIOS/internal/platform"
	"github.com/ArsinShaabani/AIOS/internal/ports"
)

func (a *API) queryFields(t *gqlTypes) graphql.Fields {
	return graphql.Fields{
		"me": &graphql.Field{Type: t.user, Resolve: func(p graphql.ResolveParams) (any, error) {
			u := userFrom(p.Context)
			if u == nil {
				return nil, platform.Unauthorized("graphql.auth", "authentication required")
			}
			return *u, nil
		}},
		"sessions": &graphql.Field{Type: graphql.NewList(t.session), Args: graphql.FieldConfigArgument{
			"limit": &graphql.ArgumentConfig{Type: graphql.Int, DefaultValue: 20},
		}, Resolve: func(p graphql.ResolveParams) (any, error) {
			u := userFrom(p.Context)
			limit, _ := p.Args["limit"].(int)
			items, _, err := a.Repos.Sessions.ListByUser(p.Context, u.ID, platform.Page{Limit: limit})
			return items, err
		}},
		"agents": &graphql.Field{Type: graphql.NewList(t.agent), Resolve: func(p graphql.ResolveParams) (any, error) {
			return a.Agents.List(p.Context, true)
		}},
		"memories": &graphql.Field{Type: graphql.NewList(t.memory), Args: graphql.FieldConfigArgument{
			"query": &graphql.ArgumentConfig{Type: graphql.String},
			"limit": &graphql.ArgumentConfig{Type: graphql.Int, DefaultValue: 10},
		}, Resolve: func(p graphql.ResolveParams) (any, error) {
			u := userFrom(p.Context)
			query, _ := p.Args["query"].(string)
			limit, _ := p.Args["limit"].(int)
			return a.Memory.Recall(p.Context, u.ID, query, limit)
		}},
		"devices": &graphql.Field{Type: graphql.NewList(t.device), Resolve: func(p graphql.ResolveParams) (any, error) {
			u := userFrom(p.Context)
			items, _, err := a.Devices.List(p.Context, u.ID, ports.DeviceFilter{}, platform.Page{Limit: 100})
			return items, err
		}},
		"overview": &graphql.Field{Type: t.overview, Resolve: func(p graphql.ResolveParams) (any, error) {
			principal := principalFrom(p.Context)
			if principal == nil || !principal.Can(securityPermUsersRead) {
				return nil, platform.Forbidden("graphql.overview", "administrator access required")
			}
			o, err := a.Repos.Stats.Overview(p.Context)
			return map[string]any{
				"users": o.Users, "sessions": o.Sessions, "messages": o.Messages,
				"devices": o.Devices, "memories": o.Memories,
			}, err
		}},
		"providers": &graphql.Field{Type: graphql.NewList(t.provider), Resolve: func(p graphql.ResolveParams) (any, error) {
			return a.Router.Providers(), nil
		}},
	}
}

func (a *API) mutationFields(t *gqlTypes) graphql.Fields {
	return graphql.Fields{
		"sendMessage": &graphql.Field{Type: t.reply, Args: graphql.FieldConfigArgument{
			"message":   &graphql.ArgumentConfig{Type: graphql.NewNonNull(graphql.String)},
			"sessionId": &graphql.ArgumentConfig{Type: graphql.String},
			"agentId":   &graphql.ArgumentConfig{Type: graphql.String},
		}, Resolve: func(p graphql.ResolveParams) (any, error) {
			u := userFrom(p.Context)
			sessionID, _ := p.Args["sessionId"].(string)
			agentID, _ := p.Args["agentId"].(string)
			out, err := a.Brain.Chat(p.Context, brain.ChatInput{
				UserID: u.ID, SessionID: sessionID, Message: p.Args["message"].(string),
				AgentID: agentID, Channel: domain.ChannelAPI,
			}, nil)
			if err != nil {
				return nil, err
			}
			return map[string]any{
				"sessionId": out.SessionID, "content": out.Content, "agentId": out.AgentID,
				"provider": out.Provider, "model": out.Model,
			}, nil
		}},
		"createMemory": &graphql.Field{Type: t.memory, Args: graphql.FieldConfigArgument{
			"content": &graphql.ArgumentConfig{Type: graphql.NewNonNull(graphql.String)},
			"kind":    &graphql.ArgumentConfig{Type: graphql.String},
		}, Resolve: func(p graphql.ResolveParams) (any, error) {
			u := userFrom(p.Context)
			kind, _ := p.Args["kind"].(string)
			m, err := a.Memory.Remember(p.Context, &domain.Memory{
				UserID: u.ID, Content: p.Args["content"].(string), Kind: domain.MemoryKind(kind), Source: "graphql",
			})
			if err != nil {
				return nil, err
			}
			return *m, nil
		}},
		"deleteMemory": &graphql.Field{Type: graphql.Boolean, Args: graphql.FieldConfigArgument{
			"id": &graphql.ArgumentConfig{Type: graphql.NewNonNull(graphql.String)},
		}, Resolve: func(p graphql.ResolveParams) (any, error) {
			u := userFrom(p.Context)
			err := a.Memory.Delete(p.Context, u.ID, p.Args["id"].(string))
			return err == nil, err
		}},
	}
}
