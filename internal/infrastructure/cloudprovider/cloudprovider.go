// Package cloudprovider abstracts over cloud-VPS APIs so the provisioning
// state machine can target multiple providers without baking Hetzner-specific
// types into the orchestration layer.
//
// One implementation today (Hetzner). The interface is intentionally small;
// adding OVH or Vultr later means implementing six methods.
package cloudprovider

import "context"

// Provider is the surface the provisioning state machine talks to.
type Provider interface {
	Name() string

	// Catalog — what's available to provision against. Used by the admin
	// dropdowns when an operator is composing a template.
	Locations(ctx context.Context) ([]Location, error)
	ServerTypes(ctx context.Context) ([]ServerType, error)
	Images(ctx context.Context) ([]Image, error)

	// Auth check, called from the admin "Test connection" button.
	Verify(ctx context.Context) error

	// Provisioning. Each returns the provider-native ID + IPv4 so the state
	// machine can record it for later cleanup.
	CreateServer(ctx context.Context, req CreateServerRequest) (*Server, error)
	DeleteServer(ctx context.Context, serverID string) error

	// Primary IP lifecycle. ipv4_per_server=1 in a template means "use the
	// IP that came with the server" — these calls are only made for extras.
	CreatePrimaryIP(ctx context.Context, req CreatePrimaryIPRequest) (*PrimaryIP, error)
	AssignPrimaryIP(ctx context.Context, ipID, serverID string) error
	UnassignPrimaryIP(ctx context.Context, ipID string) error
	DeletePrimaryIP(ctx context.Context, ipID string) error
	SetReverseDNS(ctx context.Context, ipID, hostname string) error
}

// Location is a region / datacenter where servers can be created. JSON tags
// match the admin UI's HetznerLocation type so the catalog endpoints serialize
// straight through without a translation layer.
type Location struct {
	Name        string `json:"name"`        // "fsn1", "hil", etc.
	Description string `json:"description"` // "Falkenstein DC Park 1"
	City        string `json:"city"`
	Country     string `json:"country"`      // ISO-3166 alpha-2
	Network     string `json:"network_zone"` // continent or "EU"/"US" grouping for UI
}

// ServerTypeLocationPrice is the price of one ServerType in one location.
// Hetzner prices vary per location (US regions carry a premium), so the UI
// keys the displayed price off the selected location rather than one flat
// number.
type ServerTypeLocationPrice struct {
	Location        string  `json:"location"`
	PriceMonthlyEUR float64 `json:"price_monthly_eur"`
	PriceHourlyEUR  float64 `json:"price_hourly_eur"`
}

// ServerType is one purchasable VPS shape. JSON tags are snake_case to match
// the admin UI's HetznerServerType type — without them encoding/json would
// emit PascalCase and the UI would read every field as undefined.
type ServerType struct {
	Name         string  `json:"name"` // "cx22", "cpx11"
	Description  string  `json:"description"`
	Cores        int     `json:"cores"`
	Memory       float64 `json:"memory_gb"`              // GiB
	Disk         int     `json:"disk_gb"`                // GiB
	StorageType  string  `json:"storage_type,omitempty"` // "local" / "network"
	CPUType      string  `json:"cpu_type,omitempty"`     // "shared" / "dedicated"
	Architecture string  `json:"architecture,omitempty"` // "x86" / "arm"
	// PriceMonthlyEUR is the headline (cheapest-location) gross monthly price.
	// Prices carries the full per-location breakdown the UI prefers.
	PriceMonthlyEUR float64 `json:"price_monthly_eur"`
	PriceHourlyEUR  float64 `json:"price_hourly_eur,omitempty"`
	PriceMonthlyUSD float64 `json:"price_monthly_usd,omitempty"`
	// PriceIPv4MonthlyEUR is the gross monthly cost of one extra Primary IPv4.
	// Uniform across server types (it's an IP, not a machine attribute) so it's
	// populated from a documented provider constant.
	PriceIPv4MonthlyEUR float64                   `json:"price_ipv4_monthly_eur,omitempty"`
	IncludedTrafficTB   float64                   `json:"included_traffic_tb,omitempty"`
	Prices              []ServerTypeLocationPrice `json:"prices,omitempty"`
}

// Image is an OS image available for new servers.
type Image struct {
	Name        string `json:"name"` // "ubuntu-22.04"
	Description string `json:"description"`
	OSFlavor    string `json:"os_flavor"`
	OSVersion   string `json:"os_version"`
}

// CreateServerRequest is what the state machine passes to CreateServer.
type CreateServerRequest struct {
	Name             string
	ServerType       string
	Image            string
	Location         string
	Datacenter       string // overrides Location when set
	SSHKeyIDs        []string
	UserData         string // cloud-init
	Labels           map[string]string
	PlacementGroup   string
	PrivateNetwork   string
	Firewall         string
	StartAfterCreate bool
}

// Server is what CreateServer returns.
type Server struct {
	ID         string
	Name       string
	Status     string
	PublicIPv4 string
	PublicIPv6 string
}

// CreatePrimaryIPRequest configures one extra Primary IP. The IP that
// comes free with a server is created by CreateServer, not here.
type CreatePrimaryIPRequest struct {
	Type       string // "ipv4" / "ipv6"
	Name       string
	Datacenter string // must match the server's datacenter
	Labels     map[string]string
}

// PrimaryIP is what CreatePrimaryIP returns.
type PrimaryIP struct {
	ID                 string
	Type               string
	IP                 string
	AssignedToServerID string // empty when unassigned
}
