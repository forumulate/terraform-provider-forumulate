package provider

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
)

// Plan-time validators that mirror the API's own zod schemas, so a bad value
// fails `terraform plan` instead of the apply. Every limit here is copied from
// the schema of the endpoint that owns the field; the unit tests pin them.

type stringCheck struct {
	desc string
	fn   func(s string) string // "" when valid, else why not
}

func (c stringCheck) Description(context.Context) string { return c.desc }
func (c stringCheck) MarkdownDescription(ctx context.Context) string {
	return c.Description(ctx)
}
func (c stringCheck) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	if why := c.fn(req.ConfigValue.ValueString()); why != "" {
		resp.Diagnostics.AddAttributeError(req.Path, "Invalid value", why)
	}
}

// oneOf accepts only the listed values (an enum in the API).
func oneOf(values ...string) validator.String {
	return stringCheck{
		desc: "one of: " + strings.Join(values, ", "),
		fn: func(s string) string {
			for _, v := range values {
				if s == v {
					return ""
				}
			}
			if len(values) > 24 {
				return fmt.Sprintf("%q is not accepted: it is not one of the %d values the API publishes (see the resource documentation).", s, len(values))
			}
			return fmt.Sprintf("%q is not accepted. One of: %s.", s, strings.Join(values, ", "))
		},
	}
}

// length bounds a string in characters, as zod's min/max do.
func length(min, max int) validator.String {
	return stringCheck{
		desc: fmt.Sprintf("%d to %d characters", min, max),
		fn: func(s string) string {
			n := utf8.RuneCountInString(s)
			if n < min || (max > 0 && n > max) {
				if max > 0 {
					return fmt.Sprintf("must be %d to %d characters; this is %d.", min, max, n)
				}
				return fmt.Sprintf("must be at least %d characters; this is %d.", min, n)
			}
			return ""
		},
	}
}

// pattern requires a match, with the API's own pattern and a plain reason.
func pattern(re *regexp.Regexp, reason string) validator.String {
	return stringCheck{
		desc: reason,
		fn: func(s string) string {
			if !re.MatchString(s) {
				return fmt.Sprintf("%q %s", s, reason)
			}
			return ""
		},
	}
}

var (
	slugRE            = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)
	uuidRE            = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
	localeRE          = regexp.MustCompile(`^[a-z]{2,3}(-[a-z0-9]{2,8})*$`)
	hexRE             = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)
	metaKey           = regexp.MustCompile(`^[A-Za-z0-9_.:-]{1,64}$`)
	refRE             = regexp.MustCompile(`^[\x21-\x7e]{1,200}$`)
	emailRE           = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)
	regexpLocale      = regexp.MustCompile(`^[A-Za-z]{2,3}(-[A-Za-z0-9]{2,8})*$`)
	localeOrDefaultRE = regexp.MustCompile(`^(default|[a-z]{2,3}(-[a-z0-9]{2,8})*)$`)
)

const slugReason = "must be lowercase letters and digits separated by single hyphens, like my-docs."

// slug validates the API's slug shape and length.
func slug(min, max int) []validator.String {
	return []validator.String{length(min, max), pattern(slugRE, slugReason)}
}

// uuid requires a UUID (an id the API returned, not a name).
func uuid() validator.String {
	return pattern(uuidRE, "must be a UUID.")
}

// idOrSlug accepts the API's "IdOrSlug": a UUID or a slug.
func idOrSlug() validator.String {
	return stringCheck{
		desc: "a UUID or a slug",
		fn: func(s string) string {
			if uuidRE.MatchString(s) || slugRE.MatchString(s) {
				return ""
			}
			return fmt.Sprintf("%q must be a UUID or a slug (lowercase letters, digits and hyphens).", s)
		},
	}
}

// locale is a BCP-47-ish code the API accepts for a space's languages.
func locale() validator.String {
	return pattern(localeRE, "must be a language code like en, es or pt-br (lowercase).")
}

// hexColor is a theme token colour: six-digit hex with the leading #.
func hexColor() validator.String {
	return pattern(hexRE, "must be a six-digit hex colour with the leading #, like #0b835b.")
}

// email is a loose address check; the API's zod is stricter and says so on apply.
func email() validator.String {
	return pattern(emailRE, "must be an email address.")
}

// httpURL requires an absolute http(s) URL; https=true refuses http.
func httpURL(maxLen int, httpsOnly bool) validator.String {
	return stringCheck{
		desc: "an absolute URL",
		fn: func(s string) string {
			if maxLen > 0 && len(s) > maxLen {
				return fmt.Sprintf("must be at most %d characters.", maxLen)
			}
			u, err := url.Parse(s)
			if err != nil || u.Host == "" || (u.Scheme != "https" && (httpsOnly || u.Scheme != "http")) {
				if httpsOnly {
					return fmt.Sprintf("%q must be an absolute https:// URL.", s)
				}
				return fmt.Sprintf("%q must be an absolute http:// or https:// URL.", s)
			}
			return ""
		},
	}
}

// hostname checks a DNS name (3 to 253 characters, labels of letters, digits
// and hyphens), what the domain endpoints accept.
func hostname() validator.String {
	re := regexp.MustCompile(`^(?i:[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?)(?:\.(?i:[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?))+$`)
	return stringCheck{
		desc: "a hostname such as docs.example.com",
		fn: func(s string) string {
			if len(s) < 3 || len(s) > 253 || !re.MatchString(s) {
				return fmt.Sprintf("%q must be a hostname like docs.example.com (no scheme, no path).", s)
			}
			return ""
		},
	}
}

// externalRef is the API's printable-ASCII idempotency/reference string.
func externalRef() []validator.String {
	return []validator.String{pattern(refRE, "must be 1 to 200 printable ASCII characters with no spaces.")}
}

// ---- integers ----

type intOneOf struct{ vals []int64 }

func (v intOneOf) Description(context.Context) string             { return fmt.Sprintf("one of %v", v.vals) }
func (v intOneOf) MarkdownDescription(ctx context.Context) string { return v.Description(ctx) }
func (v intOneOf) ValidateInt64(_ context.Context, req validator.Int64Request, resp *validator.Int64Response) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	for _, x := range v.vals {
		if x == req.ConfigValue.ValueInt64() {
			return
		}
	}
	resp.Diagnostics.AddAttributeError(req.Path, "Invalid value", fmt.Sprintf("%d is not accepted. One of: %v.", req.ConfigValue.ValueInt64(), v.vals))
}

// oneOfInt accepts only the listed integers.
func oneOfInt(vals ...int64) validator.Int64 { return intOneOf{vals} }

type intRange struct{ min, max int64 }

func (v intRange) Description(context.Context) string {
	return fmt.Sprintf("between %d and %d", v.min, v.max)
}
func (v intRange) MarkdownDescription(ctx context.Context) string { return v.Description(ctx) }
func (v intRange) ValidateInt64(_ context.Context, req validator.Int64Request, resp *validator.Int64Response) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	if n := req.ConfigValue.ValueInt64(); n < v.min || n > v.max {
		resp.Diagnostics.AddAttributeError(req.Path, "Invalid value",
			fmt.Sprintf("must be between %d and %d; this is %d.", v.min, v.max, n))
	}
}

// between bounds an integer, inclusive.
func between(min, max int64) validator.Int64 { return intRange{min, max} }

// ---- collections ----

type listCheck struct {
	desc    string
	min     int
	max     int
	element func(s string) string
}

func (v listCheck) Description(context.Context) string { return v.desc }
func (v listCheck) MarkdownDescription(ctx context.Context) string {
	return v.desc
}
func (v listCheck) check(path path.Path, elems []attr.Value, d *diag.Diagnostics) {
	if len(elems) < v.min || (v.max > 0 && len(elems) > v.max) {
		if v.max > 0 {
			d.AddAttributeError(path, "Invalid number of items", fmt.Sprintf("must have %d to %d items; this has %d.", v.min, v.max, len(elems)))
		} else {
			d.AddAttributeError(path, "Invalid number of items", fmt.Sprintf("must have at least %d items; this has %d.", v.min, len(elems)))
		}
	}
	if v.element == nil {
		return
	}
	for _, e := range elems {
		if s, ok := e.(basetypes.StringValue); ok && !s.IsUnknown() && !s.IsNull() {
			if why := v.element(s.ValueString()); why != "" {
				d.AddAttributeError(path, "Invalid item", why)
			}
		}
	}
}

// listOf validates a list of strings: its size and each item.
func listOf(min, max int, each ...validator.String) validator.List {
	return listValidator{listCheck{desc: "list of strings", min: min, max: max, element: eachFn(each)}}
}

// setOf validates a set of strings the same way.
func setOf(min, max int, each ...validator.String) validator.Set {
	return setValidator{listCheck{desc: "set of strings", min: min, max: max, element: eachFn(each)}}
}

func eachFn(each []validator.String) func(string) string {
	if len(each) == 0 {
		return nil
	}
	return func(s string) string {
		for _, e := range each {
			var resp validator.StringResponse
			e.ValidateString(context.Background(), validator.StringRequest{ConfigValue: basetypes.NewStringValue(s)}, &resp)
			if resp.Diagnostics.HasError() {
				return resp.Diagnostics.Errors()[0].Detail()
			}
		}
		return ""
	}
}

type listValidator struct{ listCheck }

func (v listValidator) ValidateList(_ context.Context, req validator.ListRequest, resp *validator.ListResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	v.check(req.Path, req.ConfigValue.Elements(), &resp.Diagnostics)
}

type setValidator struct{ listCheck }

func (v setValidator) ValidateSet(_ context.Context, req validator.SetRequest, resp *validator.SetResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	v.check(req.Path, req.ConfigValue.Elements(), &resp.Diagnostics)
}

// mapOfScalars validates a map(string) the API holds as metadata: key shape and
// value length.
type mapCheck struct {
	keyRE  *regexp.Regexp
	keyMsg string
	maxVal int
	maxLen int
}

func (v mapCheck) Description(context.Context) string { return "map with API-shaped keys" }
func (v mapCheck) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}
func (v mapCheck) ValidateMap(_ context.Context, req validator.MapRequest, resp *validator.MapResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	elems := req.ConfigValue.Elements()
	if v.maxLen > 0 && len(elems) > v.maxLen {
		resp.Diagnostics.AddAttributeError(req.Path, "Too many entries", fmt.Sprintf("at most %d entries; this has %d.", v.maxLen, len(elems)))
	}
	for k, e := range elems {
		if v.keyRE != nil && !v.keyRE.MatchString(k) {
			resp.Diagnostics.AddAttributeError(req.Path, "Invalid key", fmt.Sprintf("key %q %s", k, v.keyMsg))
		}
		if s, ok := e.(basetypes.StringValue); ok && !s.IsUnknown() && !s.IsNull() && v.maxVal > 0 && len(s.ValueString()) > v.maxVal {
			resp.Diagnostics.AddAttributeError(req.Path, "Value too long", fmt.Sprintf("the value of %q must be at most %d characters.", k, v.maxVal))
		}
	}
}

// metadataMap is the API's metadata: keys of [A-Za-z0-9_.:-], 1 to 64
// characters; values up to 500 characters.
func metadataMap() validator.Map {
	return mapCheck{keyRE: metaKey, keyMsg: "must be 1 to 64 characters of letters, digits and _ . : -", maxVal: 500, maxLen: 50}
}
