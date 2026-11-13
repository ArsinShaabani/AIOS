package httpapi

import (
	"net/http"

	"github.com/graphql-go/graphql"

	"github.com/ArsinShaabani/AIOS/internal/platform"
)

func (a *API) handleGraphQL(w http.ResponseWriter, r *http.Request) {
	schema, err := a.schema()
	if err != nil {
		a.fail(w, r, err)
		return
	}
	var req struct {
		Query         string         `json:"query"`
		Variables     map[string]any `json:"variables"`
		OperationName string         `json:"operationName"`
	}
	if err := decodeJSON(w, r, &req); err != nil {
		a.fail(w, r, err)
		return
	}
	if req.Query == "" {
		a.fail(w, r, platform.Invalid("graphql.query", "a GraphQL query is required"))
		return
	}
	result := graphql.Do(graphql.Params{
		Schema: schema, RequestString: req.Query, VariableValues: req.Variables,
		OperationName: req.OperationName, Context: r.Context(),
	})
	status := http.StatusOK
	if len(result.Errors) > 0 {
		status = http.StatusBadRequest
	}
	writeJSON(w, status, result)
}

func apiInvalidQuery() error {
	return &queryError{}
}

type queryError struct{}

func (e *queryError) Error() string { return "a GraphQL query is required" }

func (a *API) handleGraphQLPage(w http.ResponseWriter, r *http.Request) {
	html := `<!doctype html><html><head><meta charset="utf-8"><title>AIOS GraphQL</title>
<style>body{font-family:system-ui;margin:2rem;max-width:900px;background:#0f1115;color:#e6e6e6}
textarea{width:100%;height:180px;font-family:ui-monospace,monospace;background:#171a21;color:#e6e6e6;border:1px solid #2a2f3a;padding:.75rem;border-radius:8px}
button{margin:.75rem 0;padding:.5rem 1rem;background:#4f8ef7;color:#fff;border:0;border-radius:6px;cursor:pointer}
pre{background:#171a21;padding:1rem;overflow:auto;border-radius:8px}</style></head><body>
<h1>AIOS GraphQL</h1><p>POST queries to <code>/graphql</code> with an Authorization header.</p>
<textarea id="q">query { me { email role } agents { name } sessions(limit: 5) { title messageCount } }</textarea>
<button onclick="run()">Run</button><pre id="o"></pre>
<script>
async function run(){
 const token = localStorage.getItem("aios_token") || "";
 const res = await fetch("/graphql",{method:"POST",headers:{"Content-Type":"application/json","Authorization":"Bearer "+token},body:JSON.stringify({query:document.getElementById("q").value})});
 document.getElementById("o").textContent = JSON.stringify(await res.json(),null,2);
}
</script></body></html>`
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(html))
}
