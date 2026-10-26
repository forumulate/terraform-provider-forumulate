package e2e

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"testing"
)

// fakeForumulate is a small stateful stand-in for the Forumulate API: enough
// of spaces, collections, pages, forum categories, webhooks, domains,
// interface strings, organization settings and the manifest to run a real
// `terraform apply` against, then plan again and expect nothing to change. It
// answers the shapes the real API documents (`{space, data}` on a create, rows
// under `data` on a list, the error envelope) and rewrites Markdown the way the
// real API normalises it, so the provider's drift handling is exercised.
type fakeForumulate struct {
	t   *testing.T
	srv *httptest.Server
	mu  sync.Mutex
	n   int

	spaces      map[string]map[string]any // by id
	collections map[string]map[string]any // by id (spaceId in "_space")
	pages       map[string]map[string]any
	bodies      map[string]string // page id -> markdown as stored (normalised)
	metadata    map[string]map[string]any
	categories  map[string]map[string]any
	webhooks    map[string]map[string]any
	domains     map[string]map[string]any
	overrides   map[string]string // locale/key -> value
	settings    map[string]any    // org setting overrides
	manifestRan int
	requests    []string

	locales   map[string]map[string]any
	doors     map[string]map[string]any // "<space id>/<door>" -> document
	redirects map[string][]any
	sites     map[string]map[string]any
	connector map[string]any

	spaceConnectors map[string]map[string]any
}

// door is the stored document of one settings door of a space.
func (f *fakeForumulate) door(sp map[string]any, name string) map[string]any {
	key := fmt.Sprint(sp["id"], "/", name)
	if f.doors[key] == nil {
		f.doors[key] = map[string]any{}
	}
	return f.doors[key]
}

func (f *fakeForumulate) site(ref string) map[string]any {
	if s, ok := f.sites[ref]; ok {
		return s
	}
	for _, s := range f.sites {
		if s["slug"] == ref {
			return s
		}
	}
	return nil
}

func (f *fakeForumulate) connectorOut() map[string]any {
	if f.connector == nil {
		return map[string]any{"provider": "zoom", "name": "Zoom", "enabled": false, "configured": false, "connected": false, "connectedAccount": nil,
			"lastError": nil, "callbackUrl": "https://forumulate.test/cb", "secretsSet": []any{}, "updatedAt": nil, "values": map[string]any{}}
	}
	secrets := []any{}
	for k := range f.connector["secrets"].(map[string]any) {
		secrets = append(secrets, k)
	}
	return map[string]any{"provider": "zoom", "name": "Zoom", "enabled": f.connector["enabled"], "configured": len(secrets) > 0, "connected": false,
		"connectedAccount": nil, "lastError": nil, "callbackUrl": "https://forumulate.test/cb", "secretsSet": secrets, "updatedAt": "2027-01-07T00:00:00Z",
		"values": f.connector["values"]}
}

func orList(v any) any {
	if v == nil {
		return []any{}
	}
	return v
}

func newFake(t *testing.T) *fakeForumulate {
	f := &fakeForumulate{
		t: t, spaces: map[string]map[string]any{}, collections: map[string]map[string]any{}, pages: map[string]map[string]any{},
		bodies: map[string]string{}, metadata: map[string]map[string]any{}, categories: map[string]map[string]any{},
		webhooks: map[string]map[string]any{}, domains: map[string]map[string]any{}, overrides: map[string]string{}, settings: map[string]any{},
		locales: map[string]map[string]any{}, doors: map[string]map[string]any{}, redirects: map[string][]any{}, sites: map[string]map[string]any{}, spaceConnectors: map[string]map[string]any{},
	}
	mux := http.NewServeMux()
	f.routes(mux)
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.requests = append(f.requests, r.Method+" "+r.URL.Path)
		if r.Header.Get("Authorization") != "Bearer fp_e2e" {
			writeErr(w, 401, "auth.unauthorized", "Missing or invalid token.", "")
			return
		}
		if r.Header.Get("Forumulate-Version") != "2027-02-01" {
			writeErr(w, 400, "version.missing", "The provider must pin Forumulate-Version.", "")
			return
		}
		mux.ServeHTTP(w, r)
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeForumulate) id() string {
	f.n++
	return fmt.Sprintf("00000000-0000-4000-8000-%012d", f.n)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, code, msg, param string) {
	e := map[string]any{"code": code, "message": msg}
	if param != "" {
		e["param"] = param
	}
	writeJSON(w, status, map[string]any{"error": e, "requestId": "req_e2e"})
}

func body(r *http.Request) map[string]any {
	raw, _ := io.ReadAll(r.Body)
	var m map[string]any
	_ = json.Unmarshal(raw, &m)
	if m == nil {
		m = map[string]any{}
	}
	return m
}

func copyMap(m map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range m {
		out[k] = v
	}
	return out
}

func list(m map[string]map[string]any, pick func(map[string]any) bool) []any {
	ids := make([]string, 0, len(m))
	for id := range m {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	out := []any{}
	for _, id := range ids {
		if pick == nil || pick(m[id]) {
			out = append(out, m[id])
		}
	}
	return out
}

func page(rows []any) map[string]any {
	return map[string]any{"data": rows, "page": map[string]any{"limit": 100, "hasMore": false, "nextCursor": nil}}
}

// space looks a space up by id or slug.
func (f *fakeForumulate) space(ref string) map[string]any {
	if s, ok := f.spaces[ref]; ok {
		return s
	}
	for _, s := range f.spaces {
		if s["slug"] == ref {
			return s
		}
	}
	return nil
}

// normalise is the Markdown rewriting the real API does: "* " bullets become "- ".
func normalise(md string) string {
	lines := strings.Split(md, "\n")
	for i, l := range lines {
		if strings.HasPrefix(l, "* ") {
			lines[i] = "- " + l[2:]
		}
	}
	return strings.TrimSpace(strings.Join(lines, "\n")) + "\n"
}

func (f *fakeForumulate) routes(mux *http.ServeMux) {
	const org = "/orgs/o1"

	// ---- organization ----
	mux.HandleFunc("GET "+org, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]any{"organization": map[string]any{"id": "o1", "name": "Acme", "slug": "acme", "createdAt": "2027-01-01T00:00:00Z"}})
	})

	// ---- spaces ----
	mux.HandleFunc("POST "+org+"/spaces", func(w http.ResponseWriter, r *http.Request) {
		b := body(r)
		for _, s := range f.spaces {
			if s["slug"] == b["slug"] {
				writeErr(w, 409, "space.slug_taken", "That slug is taken.", "slug")
				return
			}
		}
		if vis, _ := b["visibility"].(string); vis == "" {
			b["visibility"] = "PRIVATE"
		}
		s := map[string]any{"id": f.id(), "type": b["type"], "slug": b["slug"], "name": b["name"], "visibility": b["visibility"],
			"settings": map[string]any{}, "createdAt": "2027-01-02T00:00:00Z", "itemCount": 0, "itemsUpdatedAt": nil, "url": nil}
		f.spaces[s["id"].(string)] = s
		writeJSON(w, 201, map[string]any{"space": s, "data": s})
	})
	mux.HandleFunc("GET "+org+"/spaces/{id}", func(w http.ResponseWriter, r *http.Request) {
		s := f.space(r.PathValue("id"))
		if s == nil {
			writeErr(w, 404, "space.not_found", "No such space.", "spaceId")
			return
		}
		writeJSON(w, 200, map[string]any{"space": s})
	})
	mux.HandleFunc("PATCH "+org+"/spaces/{id}", func(w http.ResponseWriter, r *http.Request) {
		s := f.space(r.PathValue("id"))
		if s == nil {
			writeErr(w, 404, "space.not_found", "No such space.", "spaceId")
			return
		}
		b := body(r)
		for _, k := range []string{"name", "visibility"} {
			if v, ok := b[k]; ok {
				s[k] = v
			}
		}
		settings := s["settings"].(map[string]any)
		if tr, ok := b["translations"]; ok {
			if tr == nil {
				delete(settings, "translations")
			} else {
				settings["translations"] = tr
			}
		}
		if st, ok := b["settings"].(map[string]any); ok {
			for k, v := range st {
				if v == nil {
					delete(settings, k)
				} else {
					settings[k] = v
				}
			}
		}
		writeJSON(w, 200, map[string]any{"space": s})
	})
	mux.HandleFunc("DELETE "+org+"/spaces/{id}", func(w http.ResponseWriter, r *http.Request) {
		s := f.space(r.PathValue("id"))
		if s == nil {
			writeErr(w, 404, "space.not_found", "No such space.", "spaceId")
			return
		}
		delete(f.spaces, s["id"].(string))
		writeJSON(w, 200, map[string]any{"ok": true})
	})
	mux.HandleFunc("DELETE "+org+"/spaces/{id}/destroy", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]any{"ok": true})
	})

	// ---- collections ----
	mux.HandleFunc("POST "+org+"/spaces/{sp}/collections", func(w http.ResponseWriter, r *http.Request) {
		sp := f.space(r.PathValue("sp"))
		b := body(r)
		slug, _ := b["slug"].(string)
		if slug == "" {
			slug = strings.ToLower(strings.ReplaceAll(fmt.Sprint(b["name"]), " ", "-"))
		}
		c := map[string]any{"id": f.id(), "_space": sp["id"], "name": b["name"], "slug": slug, "description": b["description"], "coverUrl": nil,
			"parentId": b["parentId"], "isDefault": false, "sortKey": "a", "nav": orEmpty(b["nav"]), "seo": orEmpty(b["seo"]), "translations": orEmpty(b["translations"])}
		f.collections[c["id"].(string)] = c
		writeJSON(w, 201, map[string]any{"collection": c, "data": c})
	})
	mux.HandleFunc("GET "+org+"/spaces/{sp}/collections", func(w http.ResponseWriter, r *http.Request) {
		sp := f.space(r.PathValue("sp"))
		writeJSON(w, 200, page(list(f.collections, func(c map[string]any) bool { return c["_space"] == sp["id"] })))
	})
	mux.HandleFunc("PATCH "+org+"/spaces/{sp}/collections/{id}", func(w http.ResponseWriter, r *http.Request) {
		c, ok := f.collections[r.PathValue("id")]
		if !ok {
			writeErr(w, 404, "not_found", "No such collection.", "collectionId")
			return
		}
		for k, v := range body(r) {
			if k == "nav" {
				nav := c["nav"].(map[string]any)
				for nk, nv := range v.(map[string]any) {
					if nv == nil {
						delete(nav, nk)
					} else {
						nav[nk] = nv
					}
				}
				continue
			}
			c[k] = v
		}
		writeJSON(w, 200, map[string]any{"collection": c})
	})
	mux.HandleFunc("DELETE "+org+"/spaces/{sp}/collections/{id}", func(w http.ResponseWriter, r *http.Request) {
		delete(f.collections, r.PathValue("id"))
		writeJSON(w, 200, map[string]any{"ok": true})
	})

	// ---- pages ----
	mux.HandleFunc("POST "+org+"/spaces/{sp}/collections/{col}/pages", func(w http.ResponseWriter, r *http.Request) {
		sp := f.space(r.PathValue("sp"))
		b := body(r)
		for _, p := range f.pages {
			if p["_col"] == r.PathValue("col") && p["slug"] == b["slug"] {
				writeErr(w, 409, "page.slug_taken", "A page with that slug exists in this collection.", "slug")
				return
			}
		}
		p := map[string]any{"id": f.id(), "_space": sp["id"], "_col": r.PathValue("col"), "slug": b["slug"], "title": b["title"], "status": "draft",
			"locale": "default", "versionKey": "current", "parentId": b["parentId"], "contentHash": "h0", "hasDraft": true, "url": nil, "updatedAt": "2027-01-03T00:00:00Z"}
		f.pages[p["id"].(string)] = p
		f.metadata[p["id"].(string)] = map[string]any{}
		if md, ok := b["body"].(string); ok {
			f.bodies[p["id"].(string)] = normalise(md)
		}
		writeJSON(w, 201, map[string]any{"page": map[string]any{"id": p["id"], "slug": p["slug"], "title": p["title"]}, "data": map[string]any{"id": p["id"]}})
	})
	pageOut := func(p map[string]any) map[string]any {
		out := copyMap(p)
		out["collection"] = map[string]any{"id": p["_col"], "name": "c"}
		delete(out, "_space")
		delete(out, "_col")
		return out
	}
	mux.HandleFunc("GET "+org+"/spaces/{sp}/pages/{id}", func(w http.ResponseWriter, r *http.Request) {
		p, ok := f.pages[r.PathValue("id")]
		if !ok {
			writeErr(w, 404, "not_found", "No such page.", "pageId")
			return
		}
		writeJSON(w, 200, map[string]any{"page": pageOut(p)})
	})
	mux.HandleFunc("GET "+org+"/spaces/{sp}/pages/{id}/draft/markdown", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/markdown")
		_, _ = w.Write([]byte(f.bodies[r.PathValue("id")]))
	})
	mux.HandleFunc("PUT "+org+"/spaces/{sp}/pages/{id}/content", func(w http.ResponseWriter, r *http.Request) {
		f.bodies[r.PathValue("id")] = normalise(fmt.Sprint(body(r)["body"]))
		writeJSON(w, 200, map[string]any{"revision": map[string]any{"id": "rev"}})
	})
	mux.HandleFunc("PATCH "+org+"/spaces/{sp}/pages/{id}", func(w http.ResponseWriter, r *http.Request) {
		p := f.pages[r.PathValue("id")]
		b := body(r)
		p["title"], p["slug"] = b["title"], b["slug"]
		writeJSON(w, 200, map[string]any{"ok": true, "slug": p["slug"]})
	})
	mux.HandleFunc("POST "+org+"/spaces/{sp}/pages/{id}/publish", func(w http.ResponseWriter, r *http.Request) {
		f.pages[r.PathValue("id")]["status"] = "published"
		writeJSON(w, 200, map[string]any{"revision": map[string]any{"id": "rev"}})
	})
	mux.HandleFunc("POST "+org+"/spaces/{sp}/pages/{id}/unpublish", func(w http.ResponseWriter, r *http.Request) {
		f.pages[r.PathValue("id")]["status"] = "draft"
		writeJSON(w, 200, map[string]any{"pageId": r.PathValue("id"), "status": "DRAFT"})
	})
	mux.HandleFunc("GET "+org+"/spaces/{sp}/pages/{id}/metadata", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]any{"metadata": f.metadata[r.PathValue("id")]})
	})
	mux.HandleFunc("PATCH "+org+"/spaces/{sp}/pages/{id}/metadata", func(w http.ResponseWriter, r *http.Request) {
		md := f.metadata[r.PathValue("id")]
		for k, v := range body(r) {
			if v == nil {
				delete(md, k)
			} else {
				md[k] = v
			}
		}
		writeJSON(w, 200, map[string]any{"metadata": md})
	})
	mux.HandleFunc("DELETE "+org+"/spaces/{sp}/pages/{id}", func(w http.ResponseWriter, r *http.Request) {
		delete(f.pages, r.PathValue("id"))
		writeJSON(w, 200, map[string]any{})
	})

	// ---- forum categories ----
	mux.HandleFunc("POST "+org+"/spaces/{sp}/forum/categories", func(w http.ResponseWriter, r *http.Request) {
		sp := f.space(r.PathValue("sp"))
		b := body(r)
		slug, _ := b["slug"].(string)
		if slug == "" {
			slug = strings.ToLower(fmt.Sprint(b["name"]))
		}
		c := map[string]any{
			"id": f.id(), "_space": sp["id"], "name": b["name"], "slug": slug, "description": "", "color": nil, "colorDark": nil, "textColor": nil, "textColorDark": nil,
			"parentId": nil, "sortOrder": json.Number("0"), "defaultNotificationLevel": "normal", "minimumRequiredTags": json.Number("0"), "defaultTags": []any{}, "allowedTags": []any{},
			"defaultSort": nil, "logoUrl": nil, "backgroundUrl": nil, "numFeaturedTopics": nil, "showSubcategoryList": false, "autoCloseHours": nil, "defaultSlowModeSeconds": nil,
			"requiredTagGroups": []any{}, "navigateToFirstPost": nil, "defaultListFilter": nil, "requireTopicApproval": false, "requireReplyApproval": false, "votingEnabled": nil,
			"searchPriority": "normal", "postGuidelines": nil, "expertUserIds": []any{}, "locale": nil, "icon": nil, "emoji": nil, "style": nil, "subcategoryListStyle": nil,
			"defaultTopPeriod": nil, "sortAscending": nil, "aboutTopicId": nil, "topicKind": "discussion", "topicTemplate": nil, "translations": map[string]any{}, "topicCount": json.Number("0"),
		}
		for k, v := range b {
			if k == "color" || k == "textColor" || k == "colorDark" || k == "textColorDark" {
				// The API normalises colours: a bare hex gains its '#'.
				if s, ok := v.(string); ok && !strings.HasPrefix(s, "#") && isHex(s) && (len(s) == 3 || len(s) == 4 || len(s) == 6 || len(s) == 8) {
					v = "#" + strings.ToLower(s)
				}
			}
			c[k] = v
		}
		f.categories[c["id"].(string)] = c
		writeJSON(w, 201, map[string]any{"category": c, "data": c})
	})
	mux.HandleFunc("GET "+org+"/spaces/{sp}/forum/categories", func(w http.ResponseWriter, r *http.Request) {
		sp := f.space(r.PathValue("sp"))
		writeJSON(w, 200, page(list(f.categories, func(c map[string]any) bool { return c["_space"] == sp["id"] })))
	})
	mux.HandleFunc("PATCH "+org+"/spaces/{sp}/forum/categories/{id}", func(w http.ResponseWriter, r *http.Request) {
		c := f.categories[r.PathValue("id")]
		for k, v := range body(r) {
			c[k] = v
		}
		writeJSON(w, 200, map[string]any{"category": c})
	})
	mux.HandleFunc("DELETE "+org+"/spaces/{sp}/forum/categories/{id}", func(w http.ResponseWriter, r *http.Request) {
		delete(f.categories, r.PathValue("id"))
		writeJSON(w, 200, map[string]any{"ok": true, "topics": 0, "movedTo": nil})
	})

	// ---- webhooks ----
	mux.HandleFunc("POST "+org+"/webhooks", func(w http.ResponseWriter, r *http.Request) {
		b := body(r)
		h := map[string]any{"id": f.id(), "url": b["url"], "events": b["events"], "active": true, "format": "json", "kind": "webhook", "bodyEncoding": "json",
			"signatureVersion": "v1", "state": "healthy", "createdAt": "2027-01-04T00:00:00Z", "spaceId": b["spaceId"], "libraryId": nil, "categoryIds": nil,
			"tagIds": nil, "groupIds": nil, "contentTypes": nil, "locales": nil, "alert_contacts": []any{}, "metadata": map[string]any{}}
		for _, k := range []string{"format", "kind", "bodyEncoding", "signatureVersion"} {
			if v, ok := b[k]; ok {
				h[k] = v
			}
		}
		f.webhooks[h["id"].(string)] = h
		writeJSON(w, 201, map[string]any{"webhook": h, "data": h, "secret": "whsec_" + h["id"].(string)[30:], "secretShown": true})
	})
	mux.HandleFunc("GET "+org+"/webhooks", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, page(list(f.webhooks, nil)))
	})
	mux.HandleFunc("PATCH "+org+"/webhooks/{id}", func(w http.ResponseWriter, r *http.Request) {
		h := f.webhooks[r.PathValue("id")]
		for k, v := range body(r) {
			if k == "alertContacts" {
				h["alert_contacts"] = v
				continue
			}
			h[k] = v
		}
		writeJSON(w, 200, map[string]any{"webhook": h})
	})
	mux.HandleFunc("DELETE "+org+"/webhooks/{id}", func(w http.ResponseWriter, r *http.Request) {
		delete(f.webhooks, r.PathValue("id"))
		writeJSON(w, 200, map[string]any{"ok": true})
	})

	// ---- domains (space) ----
	mux.HandleFunc("POST "+org+"/spaces/{sp}/domains", func(w http.ResponseWriter, r *http.Request) {
		sp := f.space(r.PathValue("sp"))
		b := body(r)
		d := map[string]any{"id": f.id(), "_space": sp["id"], "hostname": b["hostname"], "basePath": "", "verified": false, "active": false, "alsoRedirectWww": false,
			"ipAllowList": []any{}, "redirectTo": nil, "createdAt": "2027-01-05T00:00:00Z",
			"verificationRecord": map[string]any{"name": "_forumulate." + fmt.Sprint(b["hostname"]), "value": "tok-1"},
			"dnsTarget":          map[string]any{"type": "CNAME", "name": b["hostname"], "value": "edge.forumulate.test"}}
		f.domains[d["id"].(string)] = d
		writeJSON(w, 201, map[string]any{"domain": d})
	})
	mux.HandleFunc("GET "+org+"/spaces/{sp}/domains", func(w http.ResponseWriter, r *http.Request) {
		sp := f.space(r.PathValue("sp"))
		writeJSON(w, 200, page(list(f.domains, func(d map[string]any) bool { return d["_space"] == sp["id"] })))
	})
	mux.HandleFunc("DELETE "+org+"/spaces/{sp}/domains/{id}", func(w http.ResponseWriter, r *http.Request) {
		delete(f.domains, r.PathValue("id"))
		writeJSON(w, 200, map[string]any{"ok": true})
	})
	mux.HandleFunc("PUT "+org+"/spaces/{sp}/domains/{id}/ip-allow-list", func(w http.ResponseWriter, r *http.Request) {
		f.domains[r.PathValue("id")]["ipAllowList"] = body(r)["ipAllowList"]
		writeJSON(w, 200, map[string]any{"ipAllowList": body(r)["ipAllowList"]})
	})
	mux.HandleFunc("POST "+org+"/spaces/{sp}/domains/{id}/verify", func(w http.ResponseWriter, r *http.Request) {
		writeErr(w, 409, "domain.not_verified", "The TXT record was not found.", "")
	})

	// ---- translation overrides ----
	mux.HandleFunc("PUT "+org+"/translation-overrides", func(w http.ResponseWriter, r *http.Request) {
		b := body(r)
		f.overrides[fmt.Sprint(b["locale"], "/", b["key"])] = fmt.Sprint(b["value"])
		writeJSON(w, 200, map[string]any{"override": b})
	})
	mux.HandleFunc("GET "+org+"/translation-overrides", func(w http.ResponseWriter, r *http.Request) {
		rows := []any{}
		for k, v := range f.overrides {
			loc, key, _ := strings.Cut(k, "/")
			if loc == r.URL.Query().Get("locale") {
				rows = append(rows, map[string]any{"locale": loc, "key": key, "value": v})
			}
		}
		writeJSON(w, 200, page(rows))
	})
	mux.HandleFunc("DELETE "+org+"/translation-overrides", func(w http.ResponseWriter, r *http.Request) {
		delete(f.overrides, r.URL.Query().Get("locale")+"/"+r.URL.Query().Get("key"))
		writeJSON(w, 200, map[string]any{"ok": true})
	})

	// ---- organization settings ----
	registry := map[string]map[string]any{
		"search.typo_tolerance": {"key": "search.typo_tolerance", "type": "boolean", "default": true},
		"trash.retention_days":  {"key": "trash.retention_days", "type": "integer", "default": json.Number("30"), "min": json.Number("1"), "max": json.Number("365")},
	}
	setting := func(key string) map[string]any {
		row := copyMap(registry[key])
		if v, ok := f.settings[key]; ok {
			row["value"], row["isDefault"] = v, false
		} else {
			row["value"], row["isDefault"] = row["default"], true
		}
		return row
	}
	mux.HandleFunc("GET "+org+"/settings/{key}", func(w http.ResponseWriter, r *http.Request) {
		if _, ok := registry[r.PathValue("key")]; !ok {
			writeErr(w, 404, "not_found", "No such setting.", "key")
			return
		}
		writeJSON(w, 200, map[string]any{"setting": setting(r.PathValue("key"))})
	})
	mux.HandleFunc("PUT "+org+"/settings/{key}", func(w http.ResponseWriter, r *http.Request) {
		f.settings[r.PathValue("key")] = body(r)["value"]
		writeJSON(w, 200, map[string]any{"setting": setting(r.PathValue("key"))})
	})
	mux.HandleFunc("DELETE "+org+"/settings/{key}", func(w http.ResponseWriter, r *http.Request) {
		delete(f.settings, r.PathValue("key"))
		writeJSON(w, 200, map[string]any{"setting": setting(r.PathValue("key"))})
	})

	f.moreRoutes(mux)
}

func orEmpty(v any) any {
	if v == nil {
		return map[string]any{}
	}
	return v
}

func isHex(s string) bool {
	for _, c := range s {
		if !strings.ContainsRune("0123456789abcdefABCDEF", c) {
			return false
		}
	}
	return true
}
