package provider

import (
	stdjson "encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

const manifestDocJSON = `{"version":2,"spaces":[{"slug":"docs","name":"Docs","type":"DOCS"}],"webhooks":[{"url":"https://h.test/x","events":["page.published"]}]}`

func planManifest(doc string) siteManifestModel {
	return siteManifestModel{
		ID: types.StringUnknown(), ManifestJSON: S(doc), TimeoutSecs: types.Int64Value(600), Pending: types.StringUnknown(),
		IDs: types.MapUnknown(types.StringType), LastApplyJSON: types.StringUnknown(), ManualJSON: types.StringUnknown(), SecretsJSON: types.StringUnknown(),
	}
}

func planSteps() []any {
	return []any{
		map[string]any{"unit": "spaces", "ref": "docs", "action": "create", "diff": map[string]any{}, "id": nil},
		map[string]any{"unit": "webhooks", "ref": "h.test", "action": "update", "id": "wh-1", "diff": map[string]any{"events": map[string]any{"from": []any{"page.created"}, "to": []any{"page.published"}}}},
		map[string]any{"unit": "orgSettings", "ref": "x", "action": "unchanged", "id": "x"},
	}
}

func TestManifestPlanShowsTheDryRunAndApplyAppliesIt(t *testing.T) {
	f := newFakeAPI(t)
	applied := false
	f.mux.HandleFunc("PUT /orgs/org1/manifest", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("apply") == "true" {
			applied = true
			_, _ = w.Write([]byte(`{"dryRun":false,"ok":true,"summary":{"applied":2},"steps":[
				{"unit":"spaces","ref":"docs","id":"sp-9","action":"create","status":"applied","readBack":"ok"},
				{"unit":"webhooks","ref":"h.test","id":"wh-1","action":"update","status":"applied","readBack":"ok","shownOnce":{"secret":"whsec_abc"}}],
				"manual":[{"kind":"dns","name":"_f.docs.example.com","type":"TXT","value":"v","for":"domains/docs.example.com"}]}`))
			return
		}
		out := map[string]any{"dryRun": true, "ok": true, "steps": planSteps(), "manual": []any{}}
		if applied {
			out["steps"] = []any{map[string]any{"unit": "spaces", "ref": "docs", "action": "unchanged", "id": "sp-9"}}
		}
		_ = jsonEncode(w, out)
	})
	d := newDriver[siteManifestModel](t, f, NewSiteManifestResource)

	// terraform plan: the dry run is run and every step is printed.
	plan := planManifest(manifestDocJSON)
	resp := resource.ModifyPlanResponse{Plan: d.plan(plan)}
	d.r.(resource.ResourceWithModifyPlan).ModifyPlan(d.ctx, resource.ModifyPlanRequest{Plan: d.plan(plan), State: d.nullState()}, &resp)
	mustNoErr(t, resp.Diagnostics)
	var text string
	for _, w := range resp.Diagnostics.Warnings() {
		text += w.Summary() + "\n" + w.Detail() + "\n"
	}
	for _, want := range []string{"Manifest plan: 1 create, 1 update", "create spaces/docs", "update webhooks/h.test (events:", "Manifest work only you can do"} {
		if want == "Manifest work only you can do" {
			continue // the dry run above has no manual items
		}
		if !strings.Contains(text, want) {
			t.Fatalf("plan output missing %q:\n%s", want, text)
		}
	}
	var planned siteManifestModel
	mustNoErr(t, resp.Plan.Get(d.ctx, &planned))
	if !planned.Pending.IsUnknown() {
		t.Fatalf("pending must be unknown when the manifest has work to do, got %v", planned.Pending)
	}
	if dry := f.find("PUT", "/orgs/org1/manifest")[0]; dry.Query != "" {
		t.Fatalf("the plan must be a dry run (no ?apply): %q", dry.Query)
	}

	// terraform apply.
	state, diags := d.create(plan)
	mustNoErr(t, diags)
	if !applied {
		t.Fatal("create must apply")
	}
	if state.ID.ValueString() != "org1" || state.Pending.ValueString() != "none" {
		t.Fatalf("state = %+v", state)
	}
	if state.IDs.Elements()["spaces/docs"].(types.String).ValueString() != "sp-9" {
		t.Fatalf("ids = %v", state.IDs)
	}
	if !strings.Contains(state.SecretsJSON.ValueString(), "whsec_abc") || !strings.Contains(state.ManualJSON.ValueString(), "_f.docs.example.com") {
		t.Fatalf("secrets %q manual %q", state.SecretsJSON.ValueString(), state.ManualJSON.ValueString())
	}
	last := f.find("PUT", "/orgs/org1/manifest")
	if q := last[len(last)-1].Query; q != "apply=true" {
		t.Fatalf("apply query = %q", q)
	}

	// A clean dry run: pending is none and nothing is printed.
	rd, _, diags := d.read(state)
	mustNoErr(t, diags)
	if rd.Pending.ValueString() != "none" {
		t.Fatalf("pending after convergence = %q", rd.Pending.ValueString())
	}
	resp2 := resource.ModifyPlanResponse{Plan: d.plan(state)}
	d.r.(resource.ResourceWithModifyPlan).ModifyPlan(d.ctx, resource.ModifyPlanRequest{Plan: d.plan(state), State: d.state(state)}, &resp2)
	if len(resp2.Diagnostics) != 0 {
		t.Fatalf("a converged manifest must plan quietly: %v", resp2.Diagnostics)
	}

	// Drift: someone edited a webhook in the console; the refresh shows it.
	applied = false
	f.mux = http.NewServeMux()
	f.on("PUT /orgs/org1/manifest", 200, func(*http.Request, map[string]any) any {
		return map[string]any{"dryRun": true, "ok": true, "steps": planSteps()}
	})
	drifted, _, diags := d.read(state)
	mustNoErr(t, diags)
	if drifted.Pending.ValueString() != "1 create, 1 update" {
		t.Fatalf("drift not shown: %q", drifted.Pending.ValueString())
	}
}

func marshal(v any) ([]byte, error) { return stdjson.Marshal(v) }

func jsonEncode(w http.ResponseWriter, v any) error {
	b, err := marshal(v)
	if err != nil {
		return err
	}
	_, err = w.Write(b)
	return err
}

func TestManifestRefusedByTheAPIFailsThePlanOnManifestJSON(t *testing.T) {
	f := newFakeAPI(t)
	f.on("PUT /orgs/org1/manifest", 422, func(*http.Request, map[string]any) any {
		e := fail(422, "validation.failed", "Validation failed.", "spaces.0.visibility")
		e.env["details"] = map[string]any{"spaces.0.visibility": []string{"Invalid enum value."}}
		e.env["hint"] = "Visibility is PUBLIC or PRIVATE."
		return e
	})
	d := newDriver[siteManifestModel](t, f, NewSiteManifestResource)
	plan := planManifest(manifestDocJSON)
	resp := resource.ModifyPlanResponse{Plan: d.plan(plan)}
	d.r.(resource.ResourceWithModifyPlan).ModifyPlan(d.ctx, resource.ModifyPlanRequest{Plan: d.plan(plan), State: d.nullState()}, &resp)
	mustErr(t, resp.Diagnostics, "spaces.0.visibility")
	mustErr(t, resp.Diagnostics, "Visibility is PUBLIC or PRIVATE.")
	if p := resp.Diagnostics.Errors()[0]; !strings.Contains(p.Summary(), "planning the manifest") {
		t.Fatalf("summary = %q", p.Summary())
	}
}

func TestManifestPartialApplyKeepsWhatLanded(t *testing.T) {
	f := newFakeAPI(t)
	f.on("PUT /orgs/org1/manifest", 200, func(*http.Request, map[string]any) any {
		return map[string]any{"dryRun": false, "ok": false, "steps": []any{
			map[string]any{"unit": "spaces", "ref": "docs", "id": "sp-9", "action": "create", "status": "applied", "readBack": "ok"},
			map[string]any{"unit": "domains", "ref": "docs.example.com", "id": nil, "action": "create", "status": "failed", "error": "hostname is taken"},
		}}
	})
	d := newDriver[siteManifestModel](t, f, NewSiteManifestResource)
	state, diags := d.create(planManifest(manifestDocJSON))
	mustErr(t, diags, "partly applied")
	mustErr(t, diags, "domains/docs.example.com: hostname is taken")
	if state.ID.ValueString() != "org1" {
		// The framework keeps the state of a failed create when the resource
		// says it exists (tainted): the apply is resumable by applying again.
		t.Fatalf("a partly applied manifest must be kept in state, got %+v", state)
	}
}

func TestManifestEdgeCutRecoversByReplanning(t *testing.T) {
	f := newFakeAPI(t)
	f.mux.HandleFunc("PUT /orgs/org1/manifest", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("apply") == "true" {
			w.WriteHeader(504)
			_, _ = w.Write([]byte("<html>gateway timeout</html>"))
			return
		}
		_ = jsonEncode(w, map[string]any{"dryRun": true, "ok": true, "steps": []any{map[string]any{"unit": "spaces", "ref": "docs", "action": "unchanged", "id": "sp-9"}}})
	})
	d := newDriver[siteManifestModel](t, f, NewSiteManifestResource)
	st, diags := d.create(planManifest(manifestDocJSON))
	mustNoErr(t, diags)
	if len(diags.Warnings()) == 0 || st.Pending.ValueString() != "none" {
		t.Fatalf("an apply the edge cut but that completed must succeed with a warning: %v", diags)
	}
}

func TestManifestDestroyPrunesOnlyWhenAsked(t *testing.T) {
	f := newFakeAPI(t)
	f.on("PUT /orgs/org1/manifest", 200, func(*http.Request, map[string]any) any {
		return map[string]any{"dryRun": false, "ok": true, "steps": []any{}}
	})
	d := newDriver[siteManifestModel](t, f, NewSiteManifestResource)
	state := planManifest(manifestDocJSON)
	state.ID, state.Pending = S("org1"), S("none")
	mustNoErr(t, d.delete(state))
	if len(f.calls) != 0 {
		t.Fatal("destroy without destroy_prunes only forgets")
	}
	state.DestroyPrunes = types.BoolValue(true)
	mustNoErr(t, d.delete(state))
	body := f.last().Body
	if body["prune"] != true || len(body["spaces"].([]any)) != 0 || len(body["webhooks"].([]any)) != 0 {
		t.Fatalf("destroy body = %v", body)
	}
}

func TestManifestValidation(t *testing.T) {
	d := newDriver[siteManifestModel](t, newFakeAPI(t), NewSiteManifestResource)
	ok := func(doc string) bool { return len(d.attrErrors("manifest_json", S(doc))) == 0 }
	if !ok(manifestDocJSON) {
		t.Error("a valid manifest")
	}
	for name, doc := range map[string]string{
		"not json":      `{`,
		"array":         `[]`,
		"version 1":     `{"version":1}`,
		"no version":    `{"spaces":[]}`,
		"unknown key":   `{"version":2,"spaceez":[]}`,
		"bad type":      `{"version":2,"spaces":[{"slug":"a-b","name":"x","type":"WIKI"}]}`,
		"bad slug":      `{"version":2,"spaces":[{"slug":"A","name":"x","type":"DOCS"}]}`,
		"no name":       `{"version":2,"spaces":[{"slug":"ab","type":"DOCS"}]}`,
		"visibility":    `{"version":2,"spaces":[{"slug":"ab","name":"x","type":"DOCS","visibility":"HIDDEN"}]}`,
		"webhooks type": `{"version":2,"webhooks":{}}`,
	} {
		if ok(doc) {
			t.Errorf("%s must be refused", name)
		}
	}
	both := planManifest(`{"version":2,"prune":true}`)
	both.Prune = types.BoolValue(true)
	mustErr(t, d.validate(both), "set twice")
	if errs := d.attrErrors("timeout_seconds", types.Int64Value(5)); len(errs) == 0 {
		t.Error("timeout floor")
	}
	_ = diag.Diagnostics{}
}
