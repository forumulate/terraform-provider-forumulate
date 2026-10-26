package e2e

import (
	"fmt"
	"net/http"
	"strings"
)

// moreRoutes: languages, the settings doors of a space, sites, connectors and
// the manifest. Split from fakeapi_test.go to keep each file readable.
func (f *fakeForumulate) moreRoutes(mux *http.ServeMux) {
	const org = "/orgs/o1"

	// ---- languages ----
	mux.HandleFunc("POST "+org+"/spaces/{sp}/docs/locales", func(w http.ResponseWriter, r *http.Request) {
		sp := f.space(r.PathValue("sp"))
		b := body(r)
		f.locales[fmt.Sprint(sp["id"], "/", b["code"])] = map[string]any{"code": b["code"], "label": b["label"], "isDefault": false}
		writeJSON(w, 201, map[string]any{"locales": []any{}})
	})
	mux.HandleFunc("GET "+org+"/spaces/{sp}/docs/locales", func(w http.ResponseWriter, r *http.Request) {
		sp := f.space(r.PathValue("sp"))
		rows := []any{map[string]any{"code": "default", "label": "Original", "isDefault": true}}
		for k, l := range f.locales {
			if strings.HasPrefix(k, fmt.Sprint(sp["id"], "/")) {
				rows = append(rows, l)
			}
		}
		writeJSON(w, 200, page(rows))
	})
	mux.HandleFunc("PATCH "+org+"/spaces/{sp}/docs/locales/{code}", func(w http.ResponseWriter, r *http.Request) {
		sp := f.space(r.PathValue("sp"))
		if l, ok := f.locales[fmt.Sprint(sp["id"], "/", r.PathValue("code"))]; ok {
			l["label"] = body(r)["label"]
		}
		writeJSON(w, 200, map[string]any{"locales": []any{}})
	})
	mux.HandleFunc("DELETE "+org+"/spaces/{sp}/docs/locales/{code}", func(w http.ResponseWriter, r *http.Request) {
		sp := f.space(r.PathValue("sp"))
		delete(f.locales, fmt.Sprint(sp["id"], "/", r.PathValue("code")))
		writeJSON(w, 200, map[string]any{"locales": []any{}})
	})

	// ---- the settings doors of a space ----
	door := func(name string, wrap string) {
		mux.HandleFunc("GET "+org+"/spaces/{sp}/"+name, func(w http.ResponseWriter, r *http.Request) {
			sp := f.space(r.PathValue("sp"))
			doc := f.door(sp, name)
			// The API answers more than was set: defaults it fills in itself.
			out := copyMap(doc)
			out["_default"] = "added by the API"
			if wrap != "" {
				writeJSON(w, 200, map[string]any{wrap: out})
				return
			}
			writeJSON(w, 200, out)
		})
		mux.HandleFunc("PATCH "+org+"/spaces/{sp}/"+name, func(w http.ResponseWriter, r *http.Request) {
			sp := f.space(r.PathValue("sp"))
			doc := f.door(sp, name)
			for k, v := range body(r) {
				if v == nil {
					delete(doc, k)
				} else {
					doc[k] = v
				}
			}
			writeJSON(w, 200, doc)
		})
	}
	door("seo", "")
	door("theme", "theme")
	door("preset", "")
	mux.HandleFunc("PUT "+org+"/spaces/{sp}/redirects", func(w http.ResponseWriter, r *http.Request) {
		sp := f.space(r.PathValue("sp"))
		f.redirects[fmt.Sprint(sp["id"])] = body(r)["redirects"].([]any)
		writeJSON(w, 200, map[string]any{"redirects": f.redirects[fmt.Sprint(sp["id"])]})
	})
	mux.HandleFunc("GET "+org+"/spaces/{sp}/redirects", func(w http.ResponseWriter, r *http.Request) {
		sp := f.space(r.PathValue("sp"))
		rows := append([]any{}, f.redirects[fmt.Sprint(sp["id"])]...)
		rows = append(rows, map[string]any{"from": "/auto", "to": "/x", "auto": true})
		writeJSON(w, 200, page(rows))
	})

	// ---- sites ----
	mux.HandleFunc("POST "+org+"/sites", func(w http.ResponseWriter, r *http.Request) {
		b := body(r)
		cfg, _ := b["config"].(map[string]any)
		if cfg == nil {
			cfg = map[string]any{}
		}
		s := map[string]any{"id": f.id(), "slug": b["slug"], "name": b["name"], "spaceIds": orList(b["spaceIds"]), "config": cfg, "createdAt": "2027-01-06T00:00:00Z"}
		cfg["mobileNav"] = "bar" // a default the API fills in
		f.sites[s["id"].(string)] = s
		writeJSON(w, 201, map[string]any{"site": s})
	})
	mux.HandleFunc("GET "+org+"/sites/{id}", func(w http.ResponseWriter, r *http.Request) {
		s := f.site(r.PathValue("id"))
		if s == nil {
			writeErr(w, 404, "not_found", "No such site.", "siteId")
			return
		}
		writeJSON(w, 200, map[string]any{"site": s})
	})
	mux.HandleFunc("PATCH "+org+"/sites/{id}", func(w http.ResponseWriter, r *http.Request) {
		s := f.site(r.PathValue("id"))
		b := body(r)
		s["name"] = b["name"]
		if ids, ok := b["spaceIds"]; ok {
			s["spaceIds"] = ids
		}
		if cfg, ok := b["config"].(map[string]any); ok {
			cur := s["config"].(map[string]any)
			for k, v := range cfg {
				if v == nil {
					delete(cur, k)
				} else {
					cur[k] = v
				}
			}
		}
		writeJSON(w, 200, map[string]any{"site": s})
	})
	mux.HandleFunc("DELETE "+org+"/sites/{id}", func(w http.ResponseWriter, r *http.Request) {
		if s := f.site(r.PathValue("id")); s != nil {
			delete(f.sites, s["id"].(string))
		}
		writeJSON(w, 200, map[string]any{"ok": true})
	})

	// ---- space connectors ----
	mux.HandleFunc("POST "+org+"/spaces/{sp}/connectors", func(w http.ResponseWriter, r *http.Request) {
		sp := f.space(r.PathValue("sp"))
		b := body(r)
		cfg, _ := b["config"].(map[string]any)
		fields := []any{}
		for k := range cfg {
			fields = append(fields, k)
		}
		c := map[string]any{"id": f.id(), "_space": sp["id"], "providerId": b["providerId"], "providerName": "Intercom", "embedEnabled": b["embedEnabled"] == true,
			"knowledgeEnabled": b["knowledgeEnabled"] == true, "knowledgeScopes": orList(b["knowledgeScopes"]), "appearance": orEmpty(b["appearance"]),
			"configuredFields": fields, "missingFields": []any{}, "hasToken": false, "tokenPrefix": nil}
		f.spaceConnectors[c["id"].(string)] = c
		writeJSON(w, 201, map[string]any{"connector": c, "data": c})
	})
	mux.HandleFunc("GET "+org+"/spaces/{sp}/connectors", func(w http.ResponseWriter, r *http.Request) {
		sp := f.space(r.PathValue("sp"))
		writeJSON(w, 200, page(list(f.spaceConnectors, func(c map[string]any) bool { return c["_space"] == sp["id"] })))
	})
	mux.HandleFunc("PATCH "+org+"/spaces/{sp}/connectors/{id}", func(w http.ResponseWriter, r *http.Request) {
		c := f.spaceConnectors[r.PathValue("id")]
		for k, v := range body(r) {
			if k == "config" || k == "appearance" || k == "metadata" {
				continue
			}
			c[k] = v
		}
		writeJSON(w, 200, map[string]any{"connector": c})
	})
	mux.HandleFunc("POST "+org+"/spaces/{sp}/connectors/{id}/token", func(w http.ResponseWriter, r *http.Request) {
		c := f.spaceConnectors[r.PathValue("id")]
		c["hasToken"], c["tokenPrefix"] = true, "fpc_"+r.PathValue("id")[30:]
		writeJSON(w, 201, map[string]any{"token": "fpc_secret_" + r.PathValue("id")[30:], "tokenPrefix": c["tokenPrefix"], "scopes": []any{}})
	})
	mux.HandleFunc("DELETE "+org+"/spaces/{sp}/connectors/{id}/token", func(w http.ResponseWriter, r *http.Request) {
		c := f.spaceConnectors[r.PathValue("id")]
		c["hasToken"], c["tokenPrefix"] = false, nil
		writeJSON(w, 200, map[string]any{"revoked": true})
	})
	mux.HandleFunc("DELETE "+org+"/spaces/{sp}/connectors/{id}", func(w http.ResponseWriter, r *http.Request) {
		delete(f.spaceConnectors, r.PathValue("id"))
		writeJSON(w, 200, map[string]any{"deleted": true})
	})

	// ---- service connectors ----
	mux.HandleFunc("GET "+org+"/service-connectors/catalog", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, page([]any{map[string]any{"provider": "zoom", "fields": []any{
			map[string]any{"key": "clientId", "kind": "text"}, map[string]any{"key": "clientSecret", "kind": "secret"}}}}))
	})
	mux.HandleFunc("PUT "+org+"/service-connectors/zoom", func(w http.ResponseWriter, r *http.Request) {
		b := body(r)
		c := f.connector
		if c == nil {
			c = map[string]any{"values": map[string]any{}, "secrets": map[string]any{}}
			f.connector = c
		}
		if v, ok := b["enabled"]; ok {
			c["enabled"] = v
		} else if _, set := c["enabled"]; !set {
			c["enabled"] = true
		}
		for k, v := range b["values"].(map[string]any) {
			m := c["values"].(map[string]any)
			if k == "clientSecret" {
				m = c["secrets"].(map[string]any)
			}
			if v == nil {
				delete(m, k)
			} else {
				m[k] = v
			}
		}
		writeJSON(w, 200, f.connectorOut())
	})
	mux.HandleFunc("GET "+org+"/service-connectors/zoom", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, f.connectorOut())
	})
	mux.HandleFunc("DELETE "+org+"/service-connectors/zoom", func(w http.ResponseWriter, r *http.Request) {
		f.connector = nil
		writeJSON(w, 200, map[string]any{"ok": true})
	})

	// ---- manifest ----
	mux.HandleFunc("PUT "+org+"/manifest", func(w http.ResponseWriter, r *http.Request) {
		b := body(r)
		apply := r.URL.Query().Get("apply") == "true"
		if v, _ := b["version"].(float64); v != 2 {
			writeErr(w, 422, "validation.failed", "version must be 2", "version")
			return
		}
		steps := []any{}
		spaces, _ := b["spaces"].([]any)
		for _, e := range spaces {
			sp := e.(map[string]any)
			existing := f.space(fmt.Sprint(sp["slug"]))
			step := map[string]any{"unit": "spaces", "ref": sp["slug"], "id": nil, "diff": map[string]any{}}
			switch {
			case existing == nil:
				step["action"] = "create"
				if apply {
					s := map[string]any{"id": f.id(), "type": sp["type"], "slug": sp["slug"], "name": sp["name"], "visibility": "PRIVATE", "settings": map[string]any{}, "createdAt": "x"}
					f.spaces[s["id"].(string)] = s
					step["id"], step["status"], step["readBack"] = s["id"], "applied", "ok"
				}
			case existing["name"] != sp["name"]:
				step["action"], step["id"] = "update", existing["id"]
				step["diff"] = map[string]any{"name": map[string]any{"from": existing["name"], "to": sp["name"]}}
				if apply {
					existing["name"] = sp["name"]
					step["status"], step["readBack"] = "applied", "ok"
				}
			default:
				step["action"], step["id"] = "unchanged", existing["id"]
				if apply {
					step["status"] = "unchanged"
				}
			}
			steps = append(steps, step)
		}
		if apply {
			f.manifestRan++
		}
		writeJSON(w, 200, map[string]any{"dryRun": !apply, "ok": true, "steps": steps, "manual": []any{}, "summary": map[string]any{}, "units": []any{}, "extras": []any{}, "job": nil, "recommendations": []any{}})
	})
}
