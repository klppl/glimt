// Package model holds the shared domain types used across ingest and query paths.
package model

// Referrer classes.
const (
	RefDirect   = 0
	RefSearch   = 1
	RefSocial   = 2
	RefReferral = 3
	RefInternal = 4
)

// Event types.
const (
	TypePageview = 0
	TypeCustom   = 1
)

// Site is one tracked website (tenant).
type Site struct {
	ID           int64
	UUID         string
	Name         string
	Domain       string
	APIKeyHash   string
	ScriptToken  string // path component for /s/<token>.js
	CollectToken string // path component for /e/<token>
	Public       bool
	ShareToken   string
	CreatedAt    int64
}

// Event is a single enriched, IP-free analytics event ready for storage.
type Event struct {
	WebsiteID   int64
	VisitorHash []byte
	TS          int64 // unix milliseconds
	Type        int
	Name        string
	URLPath     string
	Referrer    string
	RefClass    int
	RefSource   string

	UTMSource   string
	UTMMedium   string
	UTMCampaign string
	UTMTerm     string
	UTMContent  string

	Browser      string
	BrowserVer   string
	OS           string
	Device       string
	ScreenBucket string
	Language     string
	Country      string
	Region       string

	Props map[string]string
}
