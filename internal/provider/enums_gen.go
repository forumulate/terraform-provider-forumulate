package provider

// Generated from packages/sdk/openapi.json (the API's own enums) by the
// maintainers' refresh script; edit the spec, not this file. The API refuses a
// value outside these lists, so the provider refuses it at plan time.

// collectionIcons are the named icons a collection's nav.icon accepts.
var collectionIcons = []string{
	"book", "book-arrow", "code", "message", "calendar", "chart", "pie-chart", "puzzle", "lock", "shield",
	"star", "bell", "search", "sparkle", "link", "cog", "clock", "heart", "image", "trophy", "medal", "crown",
	"headset", "webhook", "ticket", "board", "tracker", "template", "check-circle", "warning", "translate",
	"share", "eye", "bookmark", "hash", "chat-app", "team-chat", "upload", "pencil", "quote", "user",
	"activity", "alarm-clock", "alert-circle", "alert-triangle", "anchor", "archive", "arrow-down",
	"arrow-left", "arrow-right", "arrow-right-left", "arrow-up", "arrow-up-right", "at-sign", "atom", "award",
	"badge-check", "banknote", "bar-chart", "battery", "binary", "blocks", "bluetooth", "book-marked",
	"book-open", "bot", "box", "braces", "brain", "bug", "calendar-check", "calendar-days", "camera", "check",
	"chevron-down", "chevron-left", "chevron-right", "chevron-up", "circle", "circle-check", "circle-dot",
	"circle-x", "clipboard-list", "cloud", "cloud-download", "cloud-upload", "code-2", "coffee", "compass",
	"component", "container", "copy", "corner-down-right", "cpu", "credit-card", "database", "dollar-sign",
	"download", "edit", "external-link", "eye-off", "file", "file-code", "file-json", "file-plus", "file-text",
	"files", "filter", "fingerprint", "flag", "flask-conical", "folder", "folder-open", "function-square",
	"gauge", "gift", "git-branch", "git-commit", "git-fork", "git-merge", "git-pull-request", "github",
	"gitlab", "globe", "graduation-cap", "grid", "grip-vertical", "hard-drive", "heading", "headphones",
	"help-circle", "history", "home", "hourglass", "inbox", "info", "key", "key-round", "languages", "layers",
	"layout", "layout-grid", "library", "life-buoy", "lightbulb", "line-chart", "link-2", "list", "list-checks",
	"list-ordered", "lock-keyhole", "lock-open", "log-in", "log-out", "mail", "map", "map-pin", "maximize-2",
	"megaphone", "menu", "message-circle", "message-square", "mic", "milestone", "minimize-2", "minus",
	"monitor", "moon", "more-horizontal", "more-vertical", "move", "music", "navigation", "network",
	"newspaper", "notebook-pen", "octagon-alert", "package", "palette", "panel-left", "paperclip", "pause",
	"phone", "pie-chart-2", "play", "plug", "plus", "power", "printer", "quote-2", "receipt", "redo-2",
	"refresh-cw", "regex", "repeat", "rocket", "rotate-ccw", "scan-face", "scan-search", "scale", "scissors",
	"search-code", "send", "server", "settings-2", "share-2", "shield-alert", "shield-check", "shopping-cart",
	"sliders", "smartphone", "sparkles", "square", "square-terminal", "sun", "table", "tablet", "tag", "target",
	"terminal", "test-tube", "text-cursor", "thumbs-down", "thumbs-up", "ticket-check", "timer", "trash",
	"trending-up", "type", "undo-2", "user-check", "user-plus", "users", "variable", "video", "wallet", "wand",
	"wifi", "workflow", "wrench", "x", "zap", "zoom-in", "zoom-out",
}

// categoryIcons are the named icons a forum category's icon accepts.
var categoryIcons = []string{
	"book", "book-arrow", "code", "message", "calendar", "chart", "pie-chart", "puzzle", "lock", "shield",
	"star", "bell", "search", "sparkle", "link", "cog", "clock", "heart", "image", "trophy", "medal", "crown",
	"headset", "webhook", "ticket", "board", "tracker", "template", "check-circle", "warning", "translate",
	"share", "eye", "bookmark", "hash", "chat-app", "team-chat", "upload", "pencil", "quote", "user",
	"activity", "alarm-clock", "alert-circle", "alert-triangle", "anchor", "archive", "arrow-down",
	"arrow-left", "arrow-right", "arrow-right-left", "arrow-up", "arrow-up-right", "at-sign", "atom", "award",
	"badge-check", "banknote", "bar-chart", "battery", "binary", "blocks", "bluetooth", "book-marked",
	"book-open", "bot", "box", "braces", "brain", "bug", "calendar-check", "calendar-days", "camera", "check",
	"chevron-down", "chevron-left", "chevron-right", "chevron-up", "circle", "circle-check", "circle-dot",
	"circle-x", "clipboard-list", "cloud", "cloud-download", "cloud-upload", "code-2", "coffee", "compass",
	"component", "container", "copy", "corner-down-right", "cpu", "credit-card", "database", "dollar-sign",
	"download", "edit", "external-link", "eye-off", "file", "file-code", "file-json", "file-plus", "file-text",
	"files", "filter", "fingerprint", "flag", "flask-conical", "folder", "folder-open", "function-square",
	"gauge", "gift", "git-branch", "git-commit", "git-fork", "git-merge", "git-pull-request", "github",
	"gitlab", "globe", "graduation-cap", "grid", "grip-vertical", "hard-drive", "heading", "headphones",
	"help-circle", "history", "home", "hourglass", "inbox", "info", "key", "key-round", "languages", "layers",
	"layout", "layout-grid", "library", "life-buoy", "lightbulb", "line-chart", "link-2", "list", "list-checks",
	"list-ordered", "lock-keyhole", "lock-open", "log-in", "log-out", "mail", "map", "map-pin", "maximize-2",
	"megaphone", "menu", "message-circle", "message-square", "mic", "milestone", "minimize-2", "minus",
	"monitor", "moon", "more-horizontal", "more-vertical", "move", "music", "navigation", "network",
	"newspaper", "notebook-pen", "octagon-alert", "package", "palette", "panel-left", "paperclip", "pause",
	"phone", "pie-chart-2", "play", "plug", "plus", "power", "printer", "quote-2", "receipt", "redo-2",
	"refresh-cw", "regex", "repeat", "rocket", "rotate-ccw", "scan-face", "scan-search", "scale", "scissors",
	"search-code", "send", "server", "settings-2", "share-2", "shield-alert", "shield-check", "shopping-cart",
	"sliders", "smartphone", "sparkles", "square", "square-terminal", "sun", "table", "tablet", "tag", "target",
	"terminal", "test-tube", "text-cursor", "thumbs-down", "thumbs-up", "ticket-check", "timer", "trash",
	"trending-up", "type", "undo-2", "user-check", "user-plus", "users", "variable", "video", "wallet", "wand",
	"wifi", "workflow", "wrench", "x", "zap", "zoom-in", "zoom-out", "linkedin", "youtube", "discord",
	"mastodon", "instagram", "facebook", "bluesky", "website", "behance", "devto", "dribbble", "hackernews",
	"matrix", "medium", "patreon", "pinterest", "producthunt", "reddit", "rss", "slack", "spotify",
	"stackoverflow", "substack", "telegram", "threads", "tiktok", "twitch", "vimeo", "whatsapp",
}

// WebhookEvents are the events a webhook may subscribe to.
var webhookEvents = []string{
	"page.created", "page.published", "page.updated", "page.deleted", "item.stale", "assertion.failed",
	"change_request.merged", "version.created", "version.default_changed", "locale.created", "comment.created",
	"comment.moderated", "comment.resolved", "comment.deleted", "content_rule.created", "content_rule.deleted",
	"reusable.created", "reusable.updated", "reusable.deleted", "variable.set", "variable.deleted",
	"page_template.created", "page_template.deleted", "topic.created", "topic.published", "topic.edited",
	"topic.deleted", "topic.recovered", "post.created", "post.edited", "post.deleted", "post.recovered",
	"post.flagged", "topic.solved", "topic.moved", "reaction.created", "reaction.removed",
	"chat.message_created", "notification.created", "category.created", "category.updated", "category.deleted",
	"tag.created", "tag.deleted", "reviewable.created", "reviewable.updated", "user.joined", "user.suspended",
	"user.promoted", "user.silenced", "user.anonymized", "badge.granted", "group.member_added",
	"group.member_removed", "group.updated", "group.deleted", "form.submitted", "status.incident.created",
	"status.incident.updated", "status.incident.resolved", "event.published", "event.updated",
	"event.cancelled", "rsvp.created", "rsvp.updated", "rsvp.deleted", "rsvp.checked_in",
	"roadmap.status_changed", "roadmap.merged", "roadmap.shipped", "course.enrolled", "lesson.completed",
	"quiz.passed", "course.completed", "certificate.issued", "course.purchased", "course.refunded",
	"quiz.failed", "quiz.review_needed", "blog.membership.started", "blog.membership.ended",
	"blog.review_requested", "blog.reviewed", "application.created", "application.stage_changed",
	"application.offer_answered", "contact.submitted", "release.review_requested", "release.reviewed",
	"membership.started", "membership.renewed", "membership.canceled", "membership.ended", "shop.order.paid",
	"shop.order.refunded", "form.paid", "content.entry.published", "content.entry.unpublished",
	"content.entry.deleted", "content.entry.scheduled", "content.route.changed", "content.release.published",
	"content.model.changed", "content.entry.draft_saved", "content.entry.unscheduled", "content.asset.uploaded",
	"content.asset.updated", "content.asset.deleted", "content.locale.added", "content.locale.updated",
	"content.locale.removed", "domain.added", "domain.verified", "domain.failed", "domain.activated",
	"domain.removed", "site.created", "site.updated", "site.deleted", "theme.published", "theme.saved",
	"theme.deleted", "theme.component_saved", "theme.component_deleted", "theme.component_installed",
	"theme.component_uninstalled", "theme.listing_published", "theme.listing_withdrawn", "legal.published",
	"legal.removed", "legal.access_requested", "legal.privacy_requested", "space.created", "space.updated",
	"space.deleted", "space.restored", "space.purged", "snapshot.created", "snapshot.restored",
	"snapshot.deleted", "collection.created", "collection.updated", "collection.deleted", "form.created",
	"form.updated", "form.deleted", "token.created", "token.rotated", "token.revoked",
	"service_account.created", "service_account.updated", "service_account.deleted", "role.created",
	"role.updated", "role.deleted", "invite.created", "invite.revoked", "invite.accepted", "user.removed",
	"user.role_changed", "group.grant_added", "group.grant_removed", "share_link.created", "share_link.revoked",
	"webhook.created", "webhook.updated", "webhook.deleted", "webhook.secret_rotated", "webhook.disabled",
	"extension.installed", "extension.updated", "extension.uninstalled", "automation.created",
	"automation.updated", "automation.deleted", "library.created", "library.updated", "library.deleted",
	"library.member_added", "library.member_removed", "openapi.spec.registered", "openapi.spec.synced",
	"openapi.spec.removed", "openapi.changed", "sdk.published", "sso.connection_created",
	"sso.connection_updated", "sso.connection_deleted", "sso.enforcement_changed", "scim.token_created",
	"scim.token_rotated", "scim.token_revoked", "oauth_client.created", "oauth_client.secret_rotated",
	"oauth_client.deleted", "group.created", "badge_rule.created", "badge_rule.updated", "badge_rule.deleted",
	"badge_rule.ran", "session.revoked", "org.updated", "org.deleted", "org.subscribed",
	"org.subscription_changed", "org.unsubscribed", "payments.connected", "payments.updated",
	"payments.disconnected", "email_template.saved", "email_template.reset", "mail.suppression_added",
	"mail.suppression_removed", "mail_provider.connected", "mail_provider.disconnected", "connector.installed",
	"connector.updated", "connector.uninstalled", "connector.token_minted", "connector.token_revoked",
	"chat_provider.created", "chat_provider.updated", "chat_provider.deleted", "embed_host.saved",
	"embed_host.removed", "embed_feed.created", "embed_feed.updated", "embed_feed.deleted",
	"github_agent.connected", "github_agent.disconnected", "integration.installed", "integration.updated",
	"integration.uninstalled", "cloudflare.connected", "cloudflare.updated", "cloudflare.disconnected",
	"kb.changed", "status.component.created", "status.component.updated", "status.component.status_changed",
	"status.component.removed", "status.metric.created", "status.metric.updated", "status.metric.removed",
	"status.template.created", "status.template.updated", "status.template.removed", "status.translation.saved",
	"status.translation.removed", "status.inbound.created", "status.inbound.removed", "careers.ats.connected",
	"careers.ats.updated", "careers.ats.disconnected", "careers.ats.synced", "form_template.created",
	"form_template.updated", "form_template.deleted", "form_template.attached", "ai.provider.connected",
	"ai.provider.disconnected", "ai.embedding_provider.connected", "ai.embedding_provider.disconnected",
	"ai.source.added", "ai.source.removed", "translation.created", "translation.updated", "translation.deleted",
	"speaker.created", "speaker.updated", "speaker.deleted", "event_session.created", "event_session.updated",
	"event_session.deleted", "ticket_tier.created", "ticket_tier.updated", "ticket_tier.deleted",
	"ticket_order.refunded", "ticket_order.paid", "ticket_order.requested", "promo_code.created",
	"promo_code.updated", "promo_code.deleted", "course_path.created", "course_path.updated",
	"course_path.deleted", "publish.completed", "page.live", "asset.uploaded", "asset.updated", "asset.deleted",
	"asset_folder.created", "asset_folder.updated", "asset_folder.deleted", "avatar.updated", "avatar.removed",
	"custom_field.created", "custom_field.updated", "custom_field.deleted", "custom_field.values_changed",
	"reader_domain.configured", "reader_domain.verified", "reader_domain.failed", "reader_domain.removed",
	"shape.updated", "site_preview.created", "site_preview.revoked", "theme.origin_removed",
	"theme.origin_restored", "theme.component_listing_published", "theme.component_listing_withdrawn",
	"canned_reply.created", "canned_reply.updated", "canned_reply.deleted", "flag_type.created",
	"flag_type.updated", "flag_type.deleted", "reaction_palette.updated", "custom_reaction.saved",
	"custom_reaction.deleted", "custom_emoji.saved", "custom_emoji.deleted", "saved_search.created",
	"saved_search.updated", "saved_search.deleted", "member_note.created", "member_note.deleted",
	"conversation.created", "conversation.replied", "conversation.participant_added",
	"conversation.participant_removed", "relationship.muted", "relationship.unmuted", "relationship.followed",
	"relationship.unfollowed", "screening.rule_added", "screening.rule_removed", "job.completed",
	"webhook_endpoint.failing", "webhook_endpoint.disabled", "audit.event",
}

// themeKeys are the top-level keys of a space theme (PATCH .../theme); the API refuses any other.
var themeKeys = []string{
	"about", "accentColor", "ads", "announcement", "apiOrigins", "blog", "bodyFontFamily", "brandName",
	"careers", "changelog", "chrome", "colorScheme", "colorSchemeStrict", "componentInstalls", "components",
	"contact", "contentWidth", "courses", "cssSnippets", "custom", "customCss", "customFontFamily",
	"customFontUrl", "customJs", "docs", "elementStyles", "events", "faq", "faviconUrl", "font", "fontOrigins",
	"fontOriginsRemoved", "fonts", "footnotes", "forum", "frameOrigins", "headingAnchors", "headingFontFamily",
	"i18n", "imgOrigins", "layout", "legal", "logoDarkMode", "logoIncludesName", "logoMark", "logoUrl",
	"logoUrlDark", "logoUrlLight", "mediaOrigins", "mobile", "monoFontFamily", "motion", "newsroom", "notFound",
	"onThisPage", "palette", "paletteDark", "post", "primaryColor", "primaryColorDark", "roadmap",
	"schemaLogoUrl", "scriptOrigins", "settings", "settingsSchema", "showLanguageSwitcher", "status",
	"stringOverrides", "titleBand", "typography", "whiteLabel",
}

// seoKeys are the top-level keys of a space's SEO settings.
var seoKeys = []string{
	"contentLanguage", "description", "generatedTitles", "indexHidden", "indexMemberDirectory", "linkSubpages",
	"metatags", "noindex", "ogTitleSiteSuffix", "pages", "schemaType", "seoTitle", "siteName", "socialCard",
	"socialImageUrl", "titleTemplate", "twitterSite", "verification",
}

// blogSettingsKeys are the keys of a blog space's settings.
var blogSettingsKeys = []string{
	"allowAnonymous", "anonymousCaptcha", "closeCommentsAfterDays", "commentDepth", "commentNotify",
	"commentOrder", "commentsPerPage", "defaultByline", "defaultFormat", "defaultSort", "defaultTags",
	"federation", "federationReplies", "moderateAnonymous", "moderateComments", "reactionEmojis",
	"reactionsEnabled", "whoCanPost",
}

// forumSettingsKeys are the keys of a forum space's settings.
var forumSettingsKeys = []string{
	"allowAnonymous", "anonymousCaptcha", "defaultSort", "moderateAnonymous", "moderateComments", "pinScope",
	"postVotes", "reactionEmojis", "reactionsEnabled", "whoCanPost",
}

// statusSettingsKeys are the keys of a status page's settings.
var statusSettingsKeys = []string{
	"alertChannels", "autoIncident", "autoTranslate", "dayThresholds", "maintenanceNotifyEnd",
	"maintenanceNotifyStart", "maintenanceReminderHours", "maintenanceReminders", "notifySubscribers",
	"replyTo", "smsTemplate", "stateSource", "suppressAlertsInMaintenance", "uptimeExcludesMaintenance",
}

// siteConfigKeys are the keys of a site's config.
var siteConfigKeys = []string{
	"accountMenu", "ai", "align", "analytics", "brandText", "comments", "copyright", "cta", "ctaStyle", "ctas",
	"desktopNav", "discovery", "footer", "footerColumns", "footerDescription", "footerLinks", "header",
	"headerLinks", "headerSpaceIds", "height", "legalGate", "logoPhone", "mobileNav", "navMenu", "pageMounts",
	"readerIdentity", "readerThemes", "search", "searchHideOn", "searchPlacement", "searchSettings",
	"searchStyle", "secondaryCta", "seo", "showSignIn", "sidebarBrand", "sidebarToggle", "signInStyle",
	"socialLinks", "sticky", "subnav", "tabs", "tagline", "theme", "themeToggle", "topBar", "translations",
}

// pageMetadataKeys are the keys of a page's metadata.
var pageMetadataKeys = []string{
	"api", "apiLocale", "apiMethod", "apiPage", "audience", "availability", "badge", "canonicalUrl",
	"cmsBinding", "commentsEnabled", "contentLanguage", "coverUrl", "customCss", "customHead", "editUrl",
	"externalUrl", "fields", "hero", "hideCover", "hideFeedback", "hideFromNav", "hidePager", "hideTitle",
	"hideUpdated", "icon", "jsonLd", "keywords", "kind", "layout", "lesson", "navClass", "navData", "navFont",
	"navKind", "navTitle", "nofollow", "noindex", "ogDescription", "ogTitle", "onThisPage", "owner",
	"pageChrome", "pager", "related", "schemaType", "searchExclude", "seoDescription", "seoTitle",
	"shareImageUrl", "socialBadge", "sourcePath", "subtitle", "tags", "template", "theme", "themePatch",
	"titleAlign", "tocLevels", "twitterCard",
}
