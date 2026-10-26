package provider

import (
	"net/http"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

func planDomain() domainModel {
	return domainModel{
		ID: types.StringUnknown(), Hostname: S("docs.example.com"), BasePath: types.StringUnknown(),
		AlsoRedirectWWW: types.BoolUnknown(), Metadata: types.MapNull(types.StringType),
		IPAllowList: types.SetUnknown(types.StringType), Verified: types.BoolUnknown(), Active: types.BoolUnknown(),
		DNSTarget: types.ObjectUnknown(dnsTargetTypes), VerificationRecord: types.ObjectUnknown(verificationTypes), CreatedAt: types.StringUnknown(),
	}
}

func domainRow(verified, active bool) map[string]any {
	return map[string]any{
		"id": "dom-1", "hostname": "docs.example.com", "basePath": "", "verified": verified, "active": active, "alsoRedirectWww": false,
		"ipAllowList": []any{}, "redirectTo": nil, "createdAt": "2027-01-01T00:00:00Z",
		"verificationRecord": map[string]any{"name": "_forumulate.docs.example.com", "value": "tok"},
		"dnsTarget":          map[string]any{"type": "CNAME", "name": "docs.example.com", "value": "edge.forumulate.com"},
	}
}

func TestDomainCRUDOnSpace(t *testing.T) {
	f := newFakeAPI(t)
	verified, active := false, false
	f.on("POST /orgs/org1/spaces/sp-1/domains", 201, func(*http.Request, map[string]any) any {
		return map[string]any{"domain": domainRow(false, false)}
	})
	f.on("GET /orgs/org1/spaces/sp-1/domains", 200, func(*http.Request, map[string]any) any {
		return map[string]any{"data": []any{domainRow(verified, active)}}
	})
	verifyOK := false
	f.on("POST /orgs/org1/spaces/sp-1/domains/dom-1/verify", 200, func(*http.Request, map[string]any) any {
		if !verifyOK {
			return fail(409, "domain.not_verified", "The TXT record was not found.", "")
		}
		verified, active = true, true
		return map[string]any{"domain": domainRow(true, true)}
	})
	f.on("PUT /orgs/org1/spaces/sp-1/domains/dom-1/redirect", 200, func(*http.Request, map[string]any) any { return map[string]any{"ok": true} })
	f.on("PUT /orgs/org1/spaces/sp-1/domains/dom-1/ip-allow-list", 200, func(*http.Request, map[string]any) any { return map[string]any{} })
	f.on("PUT /orgs/org1/spaces/sp-1/domains/dom-1/www", 200, func(*http.Request, map[string]any) any { return map[string]any{} })
	f.on("DELETE /orgs/org1/spaces/sp-1/domains/dom-1", 200, func(*http.Request, map[string]any) any { return map[string]any{"ok": true} })

	d := newDriver[domainModel](t, f, NewDomainResource)
	plan := planDomain()
	plan.SpaceID = S("sp-1")
	plan.Activate = types.BoolValue(true)
	plan.IPAllowList = setOfStrings([]string{"10.0.0.0/8"})
	state, diags := d.create(plan)
	mustNoErr(t, diags)
	// DNS is not there yet: a warning, not a failure, and the records are exposed.
	if len(diags.Warnings()) == 0 || state.Active.ValueBool() {
		t.Fatalf("expected a not-verified warning and an inactive domain: %v", diags)
	}
	if state.DNSTarget.Attributes()["value"].(types.String).ValueString() != "edge.forumulate.com" ||
		state.VerificationRecord.Attributes()["name"].(types.String).ValueString() != "_forumulate.docs.example.com" {
		t.Fatalf("dns records not exposed: %v %v", state.DNSTarget, state.VerificationRecord)
	}
	if b := f.find("PUT", "/orgs/org1/spaces/sp-1/domains/dom-1/ip-allow-list")[0].Body; b["ipAllowList"] == nil {
		t.Fatalf("ip allow list body = %v", b)
	}

	// DNS published: the next apply (active is still false) activates.
	verifyOK = true
	up := state
	f.reset()
	state2, diags := d.update(up, state)
	mustNoErr(t, diags)
	if !state2.Active.ValueBool() || !state2.Verified.ValueBool() {
		t.Fatalf("not activated: %+v", state2)
	}
	if v := f.find("POST", "/orgs/org1/spaces/sp-1/domains/dom-1/verify")[0].Body; v["activate"] != true {
		t.Fatalf("verify body = %v", v)
	}

	// Redirect and www go through their own doors.
	mv := state2
	mv.RedirectTo = S("https://example.com/docs")
	mv.AlsoRedirectWWW = types.BoolValue(true)
	f.reset()
	_, diags = d.update(mv, state2)
	mustNoErr(t, diags)
	if b := f.find("PUT", "/orgs/org1/spaces/sp-1/domains/dom-1/redirect")[0].Body; b["redirectTo"] != "https://example.com/docs" {
		t.Fatalf("redirect body = %v", b)
	}
	if b := f.find("PUT", "/orgs/org1/spaces/sp-1/domains/dom-1/www")[0].Body; b["alsoRedirectWww"] != true {
		t.Fatalf("www body = %v", b)
	}
	clr := mv
	clr.RedirectTo = types.StringNull()
	f.reset()
	if _, diags = d.update(clr, mv); diags.HasError() {
		t.Fatal(details(diags))
	}
	if b := f.find("PUT", "/orgs/org1/spaces/sp-1/domains/dom-1/redirect")[0].Body; b["redirectTo"] != nil {
		t.Fatalf("a removed redirect_to must be null: %v", b)
	}

	mustNoErr(t, d.delete(state2))
	imp, _, diags := d.importState("space/sp-1/dom-1")
	mustNoErr(t, diags)
	if imp.Hostname.ValueString() != "docs.example.com" || imp.SpaceID.ValueString() != "sp-1" {
		t.Fatalf("import = %+v", imp)
	}
	_, _, diags = d.importState("sp-1/dom-1")
	mustErr(t, diags, "Invalid import id")
}

func TestDomainOnSiteAndValidation(t *testing.T) {
	f := newFakeAPI(t)
	f.on("POST /orgs/org1/sites/st-1/domains", 201, func(*http.Request, map[string]any) any {
		return map[string]any{"domain": domainRow(false, false)}
	})
	f.on("GET /orgs/org1/sites/st-1/domains", 200, func(*http.Request, map[string]any) any {
		return map[string]any{"data": []any{domainRow(false, false)}}
	})
	d := newDriver[domainModel](t, f, NewDomainResource)
	plan := planDomain()
	plan.SiteID = S("st-1")
	state, diags := d.create(plan)
	mustNoErr(t, diags)
	if !state.IPAllowList.IsNull() || !state.RedirectTo.IsNull() {
		t.Fatalf("a site domain has no ip_allow_list or redirect_to: %+v", state)
	}

	both := planDomain()
	both.SpaceID, both.SiteID = S("a"), S("b")
	mustErr(t, d.validate(both), "Exactly one")
	neither := planDomain()
	mustErr(t, d.validate(neither), "Exactly one")
	site := planDomain()
	site.SiteID = S("st-1")
	site.RedirectTo = S("https://x.test")
	mustErr(t, d.validate(site), "space's domain only")

	for name, c := range map[string]struct {
		v  string
		ok bool
	}{
		"ok": {"docs.example.com", true}, "scheme": {"https://docs.example.com", false}, "path": {"docs.example.com/x", false},
		"single label": {"localhost", false}, "underscore": {"do_cs.example.com", false}, "dash edge": {"-a.example.com", false},
	} {
		if errs := d.attrErrors("hostname", S(c.v)); (len(errs) == 0) != c.ok {
			t.Errorf("%s: %v", name, errs)
		}
	}
	if errs := d.attrErrors("redirect_to", S("ftp://x.test")); len(errs) == 0 {
		t.Error("redirect_to must be http(s)")
	}
}
