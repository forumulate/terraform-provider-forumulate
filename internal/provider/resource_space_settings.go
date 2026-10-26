package provider

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/forumulate/terraform-provider-forumulate/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type spaceSettingsModel struct {
	ID                 types.String `tfsdk:"id"`
	SpaceID            types.String `tfsdk:"space_id"`
	Preset             types.String `tfsdk:"preset"`
	PiecePreset        types.String `tfsdk:"piece_preset"`
	FrontDoorJSON      types.String `tfsdk:"front_door_json"`
	SEOJSON            types.String `tfsdk:"seo_json"`
	ThemeJSON          types.String `tfsdk:"theme_json"`
	BlogSettingsJSON   types.String `tfsdk:"blog_settings_json"`
	ForumSettingsJSON  types.String `tfsdk:"forum_settings_json"`
	StatusSettingsJSON types.String `tfsdk:"status_settings_json"`
	Redirects          types.List   `tfsdk:"redirects"`
}

var redirectTypes = map[string]attr.Type{
	"from": types.StringType, "to": types.StringType, "type": types.StringType, "status": types.Int64Type, "pass_query": types.BoolType,
}

// redirectPath mirrors the API's `^\/(?!\/)[^\s?#]*$` (Go's regexp has no lookahead).
func redirectPath() validator.String {
	return stringCheck{
		desc: "a path starting with a single /",
		fn: func(s string) string {
			if len(s) < 1 || len(s) > 512 || s[0] != '/' || strings.HasPrefix(s, "//") || strings.ContainsAny(s, " \t\r\n?#") {
				return fmt.Sprintf("%q must be a path of 1 to 512 characters that starts with a single / and has no spaces, ? or #.", s)
			}
			return ""
		},
	}
}

// redirectTarget is a path or an https URL.
func redirectTarget() validator.String {
	path := redirectPath()
	return stringCheck{
		desc: "a path or an https URL",
		fn: func(s string) string {
			if strings.HasPrefix(s, "https://") {
				if len(s) > 512 || strings.ContainsAny(s, " \t\r\n?#") {
					return fmt.Sprintf("%q must be at most 512 characters with no spaces, ? or #.", s)
				}
				return ""
			}
			var r validator.StringResponse
			path.ValidateString(context.Background(), validator.StringRequest{ConfigValue: types.StringValue(s)}, &r)
			if r.Diagnostics.HasError() {
				return fmt.Sprintf("%q must be a path starting with a single / or an https:// URL.", s)
			}
			return ""
		},
	}
}

// NewSpaceSettingsResource is `forumulate_space_settings`.
func NewSpaceSettingsResource() resource.Resource {
	return newResource(&resourceSpec[spaceSettingsModel]{
		suffix: "space_settings",
		what:   "space settings",
		description: "The settings doors of one space, in one resource per space: its layout (`preset`, `piece_preset`, `front_door_json`), `seo_json`, `theme_json`, the redirect table, and the type's own settings (`blog_settings_json`, `forum_settings_json`, `status_settings_json`). " +
			"Each part is optional and only the parts you declare are managed.\n\n" +
			"Every JSON part is sent as a PATCH, so it merges into what is stored and a key you remove from the configuration is cleared. Drift is detected for the keys you declare; settings someone else manages are left alone. " +
			"`redirects` replaces the operator-managed redirect table (rules the platform wrote itself on a rename are kept).\n\n" +
			"Destroying the resource resets what it declared: the JSON parts' keys (and the front door's) are cleared and the redirect table emptied. The layout names (`preset`, `piece_preset`) have no reset and stay as they are.",
		importHelp: "the space's id or slug",
		paramAlias: map[string]string{"frontDoor": "front_door_json", "piecePreset": "piece_preset"},
		attrs: map[string]schema.Attribute{
			"id":                   computedID("The space's id."),
			"space_id":             reqStr("The space (its id or slug). Changing it replaces the resource.", replStr, idOrSlug()),
			"preset":               optStr("The named layout the reader wears; validated by the API against the space's type (`GET /space-types/{type}/presets`).", length(1, 40)),
			"piece_preset":         optStr("The layout of a piece's own page (a post, a topic); validated against the type (`piecePresets`).", length(1, 40)),
			"front_door_json":      jsonAttr("The front door's switches and heading (`title`, `showFacts`, ...); the keys each type accepts are in `GET /space-types/{type}/presets` under `frontDoor`."),
			"seo_json":             jsonAttr("The space's SEO: `seoTitle`, `description`, `siteName`, `titleTemplate`, `socialImageUrl`, `socialCard`, `twitterSite`, `verification`, `noindex`, `metatags`, `pages`, ...", seoKeys...),
			"theme_json":           jsonAttr("The space's theme: colours, fonts, `components`, `chrome`, `motion`, `mobile`, ... Colour tokens are six-digit hex with the leading #. A key the theme does not have is refused by name.", themeKeys...),
			"blog_settings_json":   jsonAttr("A BLOG space's settings: `whoCanPost`, `moderateComments`, `commentOrder`, `defaultByline`, `federation`, ...", blogSettingsKeys...),
			"forum_settings_json":  jsonAttr("A FORUM space's community settings: `whoCanPost`, `moderateComments`, `postVotes`, `pinScope`, `reactionEmojis`, ...", forumSettingsKeys...),
			"status_settings_json": jsonAttr("A STATUS space's page settings: `autoIncident`, `notifySubscribers`, `alertChannels`, `dayThresholds`, `smsTemplate`, ...", statusSettingsKeys...),
			"redirects": schema.ListNestedAttribute{
				Optional:            true,
				MarkdownDescription: "The space's redirect table (up to 500 rules). Replaced whole; platform-written rename redirects are kept.",
				Validators:          []validator.List{listOf(0, 500)},
				NestedObject: schema.NestedAttributeObject{Attributes: map[string]schema.Attribute{
					"from":       schema.StringAttribute{Required: true, Validators: []validator.String{redirectPath()}, MarkdownDescription: "The old path, starting with a single `/`; a trailing `*` matches any suffix."},
					"to":         schema.StringAttribute{Required: true, Validators: []validator.String{redirectTarget()}, MarkdownDescription: "The new path, or an https URL (a redirect only; a rewrite names a path)."},
					"type":       schema.StringAttribute{Optional: true, Validators: []validator.String{oneOf("redirect", "rewrite")}, MarkdownDescription: "`redirect` (the default) sends the reader on; `rewrite` serves `to` at the requested address."},
					"status":     schema.Int64Attribute{Optional: true, Validators: []validator.Int64{oneOfInt(301, 302, 307, 308)}, MarkdownDescription: "The redirect's HTTP status: 301, 302, 307 or 308 (the default). Ignored by a rewrite."},
					"pass_query": schema.BoolAttribute{Optional: true, MarkdownDescription: "Carry the incoming query string to the target."},
				}},
			},
		},
		validate: func(ctx context.Context, cfg *spaceSettingsModel, d *diag.Diagnostics) {
			if known(cfg.Redirects) {
				for i, e := range cfg.Redirects.Elements() {
					o, ok := e.(types.Object)
					if !ok || o.IsUnknown() {
						continue
					}
					a := o.Attributes()
					typ, _ := a["type"].(types.String)
					to, _ := a["to"].(types.String)
					if typ.ValueString() == "rewrite" && known(to) && strings.HasPrefix(to.ValueString(), "https://") {
						d.AddAttributeError(attrPath("redirects"), "A rewrite cannot target another site",
							fmt.Sprintf("Rule %d is a rewrite to %q: a rewrite serves a path of this space and is not a proxy.", i+1, to.ValueString()))
					}
				}
			}
		},
		create: func(ctx context.Context, c *client.Client, m *spaceSettingsModel, d *diag.Diagnostics) error {
			m.ID = m.SpaceID
			if err := writeSpaceSettings(ctx, c, m, nil, d); err != nil {
				return err
			}
			return readSpaceSettings(ctx, c, m)
		},
		created: func(m *spaceSettingsModel) bool { return known(m.ID) },
		read: func(ctx context.Context, c *client.Client, m *spaceSettingsModel, d *diag.Diagnostics) error {
			if m.SpaceID.IsNull() || m.SpaceID.ValueString() == "" {
				m.SpaceID = m.ID
			}
			return readSpaceSettings(ctx, c, m)
		},
		update: func(ctx context.Context, c *client.Client, plan, prior *spaceSettingsModel, d *diag.Diagnostics) error {
			plan.ID = prior.ID
			if err := writeSpaceSettings(ctx, c, plan, prior, d); err != nil {
				return err
			}
			return readSpaceSettings(ctx, c, plan)
		},
		del: func(ctx context.Context, c *client.Client, m *spaceSettingsModel, d *diag.Diagnostics) error {
			// Reset what was declared: every declared top-level key is cleared.
			empty := types.StringValue("{}")
			reset := *m
			// The layout names have no reset: they stay as they are.
			reset.FrontDoorJSON = types.StringValue("{}")
			if !known(m.FrontDoorJSON) {
				reset.FrontDoorJSON = types.StringNull()
			}
			for _, p := range []*types.String{&reset.SEOJSON, &reset.ThemeJSON, &reset.BlogSettingsJSON, &reset.ForumSettingsJSON, &reset.StatusSettingsJSON} {
				if known(*p) {
					*p = empty
				}
			}
			reset.Redirects = types.ListNull(types.ObjectType{AttrTypes: redirectTypes})
			if known(m.Redirects) {
				reset.Redirects = types.ListValueMust(types.ObjectType{AttrTypes: redirectTypes}, nil)
			}
			return writeSpaceSettings(ctx, c, &reset, m, d)
		},
		importID: func(id string) (map[string]string, error) {
			return map[string]string{"id": id, "space_id": id}, nil
		},
	})
}

// settingsDoors lists the PATCH-merged JSON parts: attribute, path, and the
// key the document goes under (empty: the body is the document itself).
type settingsDoor struct {
	tf     string
	path   []string
	get    func(m *spaceSettingsModel) types.String
	set    func(m *spaceSettingsModel, v types.String)
	only   string // the space type this door exists on, "" for any
	answer string // key of the GET answer holding the document ("" = the answer)
}

var settingsDoors = []settingsDoor{
	{"seo_json", []string{"seo"}, func(m *spaceSettingsModel) types.String { return m.SEOJSON }, func(m *spaceSettingsModel, v types.String) { m.SEOJSON = v }, "", ""},
	{"theme_json", []string{"theme"}, func(m *spaceSettingsModel) types.String { return m.ThemeJSON }, func(m *spaceSettingsModel, v types.String) { m.ThemeJSON = v }, "", "theme"},
	{"blog_settings_json", []string{"blog", "settings"}, func(m *spaceSettingsModel) types.String { return m.BlogSettingsJSON }, func(m *spaceSettingsModel, v types.String) { m.BlogSettingsJSON = v }, "BLOG", "settings"},
	{"forum_settings_json", []string{"forum", "settings"}, func(m *spaceSettingsModel) types.String { return m.ForumSettingsJSON }, func(m *spaceSettingsModel, v types.String) { m.ForumSettingsJSON = v }, "FORUM", "settings"},
	{"status_settings_json", []string{"status", "settings"}, func(m *spaceSettingsModel) types.String { return m.StatusSettingsJSON }, func(m *spaceSettingsModel, v types.String) { m.StatusSettingsJSON = v }, "STATUS", ""},
}

func writeSpaceSettings(ctx context.Context, c *client.Client, m, prior *spaceSettingsModel, d *diag.Diagnostics) error {
	space := m.SpaceID.ValueString()
	var priorM spaceSettingsModel
	if prior != nil {
		priorM = *prior
	} else {
		priorM = spaceSettingsModel{
			Preset: types.StringNull(), PiecePreset: types.StringNull(), FrontDoorJSON: types.StringNull(), SEOJSON: types.StringNull(),
			ThemeJSON: types.StringNull(), BlogSettingsJSON: types.StringNull(), ForumSettingsJSON: types.StringNull(),
			StatusSettingsJSON: types.StringNull(), Redirects: types.ListNull(types.ObjectType{AttrTypes: redirectTypes}),
		}
	}
	// The layout door: preset, piecePreset and the front door in one PATCH.
	if !m.Preset.Equal(priorM.Preset) || !m.PiecePreset.Equal(priorM.PiecePreset) || !m.FrontDoorJSON.Equal(priorM.FrontDoorJSON) {
		body := Object{}
		setStr(body, "preset", m.Preset, prior != nil)
		setStr(body, "piecePreset", m.PiecePreset, prior != nil)
		mergeBody(body, "frontDoor", priorM.FrontDoorJSON, m.FrontDoorJSON)
		if len(body) > 0 {
			out, err := c.DoObject(ctx, http.MethodPatch, c.SpacePath(space, "preset"), body)
			if err != nil {
				return err
			}
			warnList(d, out)
		}
	}
	for _, door := range settingsDoors {
		cur, old := door.get(m), door.get(&priorM)
		if cur.Equal(old) {
			continue
		}
		if door.only != "" {
			if err := requireSpaceType(ctx, c, space, door.only, door.tf, d); err != nil {
				return err
			}
		}
		body := Object{}
		mergeBodyDoc(body, old, cur)
		if len(body) == 0 {
			continue
		}
		out, err := c.DoObject(ctx, http.MethodPatch, c.SpacePath(space, door.path...), body)
		if err != nil {
			return err
		}
		warnList(d, out)
	}
	if !m.Redirects.Equal(priorM.Redirects) && (known(m.Redirects) || known(priorM.Redirects)) {
		rules := []any{}
		if known(m.Redirects) {
			rules = redirectsBody(m.Redirects)
		}
		out, err := c.DoObject(ctx, http.MethodPut, c.SpacePath(space, "redirects"), Object{"redirects": rules})
		if err != nil {
			return err
		}
		warnList(d, out)
	}
	return nil
}

// mergeBodyDoc fills body with the document itself (not under a key): the
// plan's keys, and a null for each key the prior declared and the plan dropped.
func mergeBodyDoc(body Object, prior, plan types.String) {
	holder := Object{}
	mergeBody(holder, "d", prior, plan)
	if doc, ok := holder["d"].(map[string]any); ok {
		for k, v := range doc {
			body[k] = v
		}
	}
}

func requireSpaceType(ctx context.Context, c *client.Client, space, want, attrName string, d *diag.Diagnostics) error {
	out, err := c.DoObject(ctx, http.MethodGet, c.SpacePath(space), nil)
	if err != nil {
		return err
	}
	if got := getS(client.Unwrap(out, "space"), "type").ValueString(); got != "" && got != want {
		return &attrError{attr: attrName, summary: "Wrong kind of space",
			detail: fmt.Sprintf("%s applies to %s spaces; this space is %s.", attrName, want, got)}
	}
	return nil
}

func redirectsBody(l types.List) []any {
	rules := []any{}
	for _, e := range l.Elements() {
		o, ok := e.(types.Object)
		if !ok {
			continue
		}
		a := o.Attributes()
		r := map[string]any{}
		if v, ok := a["from"].(types.String); ok {
			r["from"] = v.ValueString()
		}
		if v, ok := a["to"].(types.String); ok {
			r["to"] = v.ValueString()
		}
		if v, ok := a["type"].(types.String); ok && known(v) {
			r["type"] = v.ValueString()
		}
		if v, ok := a["status"].(types.Int64); ok && known(v) {
			r["status"] = v.ValueInt64()
		}
		if v, ok := a["pass_query"].(types.Bool); ok && known(v) {
			r["passQuery"] = v.ValueBool()
		}
		rules = append(rules, r)
	}
	return rules
}

func readSpaceSettings(ctx context.Context, c *client.Client, m *spaceSettingsModel) error {
	space := m.SpaceID.ValueString()
	if known(m.Preset) || known(m.PiecePreset) || known(m.FrontDoorJSON) {
		out, err := c.DoObject(ctx, http.MethodGet, c.SpacePath(space, "preset"), nil)
		if err != nil {
			return err
		}
		if known(m.Preset) {
			m.Preset = getS(out, "preset")
		}
		if known(m.PiecePreset) {
			m.PiecePreset = getS(out, "piecePreset")
		}
		m.FrontDoorJSON = jsonFromAPI(m.FrontDoorJSON, out["frontDoor"])
	}
	for _, door := range settingsDoors {
		cur := door.get(m)
		if !known(cur) {
			door.set(m, types.StringNull())
			continue
		}
		out, err := c.DoObject(ctx, http.MethodGet, c.SpacePath(space, door.path...), nil)
		if err != nil {
			return err
		}
		var doc any = out
		if door.answer != "" {
			if inner, ok := out[door.answer]; ok {
				doc = inner
			}
		}
		door.set(m, jsonFromAPI(cur, doc))
	}
	if known(m.Redirects) {
		rows, err := c.List(ctx, c.SpacePath(space, "redirects"), "redirects", nil)
		if err != nil {
			return err
		}
		vals := []attr.Value{}
		for _, r := range rows {
			if auto, _ := r["auto"].(bool); auto {
				continue
			}
			obj, _ := types.ObjectValue(redirectTypes, map[string]attr.Value{
				"from": getS(r, "from"), "to": getS(r, "to"), "type": getS(r, "type"), "status": getI(r, "status"), "pass_query": getB(r, "passQuery"),
			})
			vals = append(vals, obj)
		}
		m.Redirects, _ = types.ListValue(types.ObjectType{AttrTypes: redirectTypes}, vals)
	}
	if m.ID.IsNull() || m.ID.IsUnknown() {
		m.ID = m.SpaceID
	}
	return nil
}
