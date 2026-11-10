package panel

// Types below mirror the payloads actually observed on the verified panel
// (3X-UI 3.10.2, 2026-10-23). They are deliberately a subset: only fields
// Veylix consumes are decoded, so an unrelated panel field can never break us.

// envelope is the wrapper every 3X-UI endpoint returns.
type envelope[T any] struct {
	Success bool   `json:"success"`
	Msg     string `json:"msg"`
	Obj     T      `json:"obj"`
}

// ServerStatus is the authenticated health payload (GET /panel/api/server/status).
// The panel caches this for ~2 s, so it must not be polled faster than ~30 s.
type ServerStatus struct {
	CPU          float64 `json:"cpu"`
	CPUCores     int     `json:"cpuCores"`
	LogicalPro   int     `json:"logicalPro"`
	PanelVersion string  `json:"panelVersion"`
	PanelGUID    string  `json:"panelGuid"`
	Uptime       int64   `json:"uptime"`
	TCPCount     int     `json:"tcpCount"`
	Mem          struct {
		Current int64 `json:"current"`
		Total   int64 `json:"total"`
	} `json:"mem"`
	Swap struct {
		Current int64 `json:"current"`
		Total   int64 `json:"total"`
	} `json:"swap"`
	Disk struct {
		Current int64 `json:"current"`
		Total   int64 `json:"total"`
	} `json:"disk"`
	NetIO struct {
		Up   int64 `json:"up"`
		Down int64 `json:"down"`
	} `json:"netIO"`
	Xray struct {
		State   string `json:"state"`
		Version string `json:"version"`
	} `json:"xray"`
	Loads []float64 `json:"loads"`
}

// InboundOption is the lightweight picker projection
// (GET /panel/api/inbounds/options) — the only inbound call Veylix uses for
// selection UI, because it skips settings/streamSettings/clientStats.
type InboundOption struct {
	ID       int    `json:"id"`
	Remark   string `json:"remark"`
	Tag      string `json:"tag"`
	Protocol string `json:"protocol"`
	Port     int    `json:"port"`
	Enable   bool   `json:"enable"`
	Network  string `json:"network"`
	Security string `json:"security"`
	SSMethod string `json:"ssMethod"`
}

// ClientTraffic is one client's counters inside clientStats.
type ClientTraffic struct {
	ID         int    `json:"id"`
	InboundID  int    `json:"inboundId"`
	Email      string `json:"email"`
	Up         int64  `json:"up"`
	Down       int64  `json:"down"`
	Total      int64  `json:"total"`
	ExpiryTime int64  `json:"expiryTime"`
	Enable     bool   `json:"enable"`
	ResetCount int    `json:"resetCount"`
	LastOnline int64  `json:"lastOnline"`
}

// ClientRow is one entry of GET /panel/api/clients/list/paged.
//
// LimitHwid is the hardware/device limit. It was missing from this struct and
// from ClientSpec, which is why device limits never applied on the panel even
// though the API accepts the field (D-028.5, ledger F-024).
type ClientRow struct {
	Email      string         `json:"email"`
	SubID      string         `json:"subId"`
	Enable     bool           `json:"enable"`
	TotalGB    int64          `json:"totalGB"`
	ExpiryTime int64          `json:"expiryTime"`
	LimitIP    int            `json:"limitIp"`
	LimitHwid  int            `json:"limitHwid"`
	InboundIDs []int          `json:"inboundIds"`
	Traffic    *ClientTraffic `json:"traffic"`
	CreatedAt  int64          `json:"createdAt"`
	UpdatedAt  int64          `json:"updatedAt"`
}

// PanelClient is the full record returned by GET /panel/api/clients/get/{email}.
//
// The READ shape deliberately differs from the write shape (ClientSpec): here
// `id` is the integer row id and the VLESS secret is `uuid`, whereas on write
// `id` carries the uuid. Converting between them is a separate, explicit step
// (see SpecFromPanelClient) so a read-modify-write can never silently corrupt
// a client — clients/update REPLACES the row rather than patching it.
type PanelClient struct {
	Email      string `json:"email"`
	SubID      string `json:"subId"`
	UUID       string `json:"uuid"`
	Password   string `json:"password"`
	Auth       string `json:"auth"`
	Flow       string `json:"flow"`
	Security   string `json:"security"`
	Comment    string `json:"comment"`
	TgID       int64  `json:"tgId"`
	TotalGB    int64  `json:"totalGB"`
	ExpiryTime int64  `json:"expiryTime"`
	LimitIP    int    `json:"limitIp"`
	LimitHwid  int    `json:"limitHwid"`
	Enable     bool   `json:"enable"`
	CreatedAt  int64  `json:"createdAt"`
	UpdatedAt  int64  `json:"updatedAt"`
	// InboundIDs comes from the wrapper (obj.inboundIds), not the client object.
	InboundIDs []int `json:"-"`
}

// Hwid is one registered device of a client
// (POST /panel/api/clients/hwids/{email}). Only a short fingerprint is
// exposed by the panel — never the full hash.
type Hwid struct {
	ID          int64  `json:"id"`
	Fingerprint string `json:"fingerprint"`
	UserAgent   string `json:"userAgent"`
	DeviceOS    string `json:"deviceOs"`
	OSVersion   string `json:"osVersion"`
	DeviceModel string `json:"deviceModel"`
	FirstSeen   int64  `json:"firstSeen"`
	LastSeen    int64  `json:"lastSeen"`
}

// Summary is the panel-computed counters block returned beside the page.
type Summary struct {
	Total         int      `json:"total"`
	Active        int      `json:"active"`
	OnlineCount   int      `json:"onlineCount"`
	DepletedCount int      `json:"depletedCount"`
	DeactiveCount int      `json:"deactiveCount"`
	ExpiringCount int      `json:"expiringCount"`
	Online        []string `json:"online"`
}

// ClientPage is the response object of GET /panel/api/clients/list/paged.
type ClientPage struct {
	Items    []ClientRow `json:"items"`
	Total    int         `json:"total"`
	Filtered int         `json:"filtered"`
	Page     int         `json:"page"`
	PageSize int         `json:"pageSize"`
	Summary  Summary     `json:"summary"`
}

// ClientSpec is the writable client body for POST /panel/api/clients/add
// and POST /panel/api/clients/update/{email}.
//
// The universal fields are required by the verified schema; when the protocol
// secrets (uuid/password/auth) are omitted the panel generates them
// server-side, so Veylix only sends them when it is setting a value on purpose
// (create with an owner-supplied credential, or a client-initiated change).
//
// LimitHwid is the hardware/device limit. Its absence was defect F-024: the
// producer set a device limit, Veylix never sent it, and the panel therefore
// never enforced one (D-028.5).
//
// UUID is serialised as "id" because that is the field name the panel expects
// for the VLESS/reality secret — note the read shape (PanelClient) uses `uuid`
// for the same value and `id` for the integer row id.
type ClientSpec struct {
	Email        string `json:"email"`
	SubID        string `json:"subId"`
	UUID         string `json:"id,omitempty"`
	Password     string `json:"password,omitempty"`
	Auth         string `json:"auth,omitempty"`
	Flow         string `json:"flow,omitempty"`
	Enable       bool   `json:"enable"`
	TotalGB      int64  `json:"totalGB"`
	ExpiryTime   int64  `json:"expiryTime"`
	LimitIP      int    `json:"limitIp"`
	LimitHwid    int    `json:"limitHwid"`
	Reset        int    `json:"reset"`
	ResetDay     int    `json:"resetDay"`
	ResetMax     int    `json:"resetMax"`
	ResetWeekday int    `json:"resetWeekday"`
	Security     string `json:"security"`
	TgID         int64  `json:"tgId"`
	Comment      string `json:"comment"`
}

// SpecFromPanelClient converts a read record into the write payload.
//
// This exists because clients/update REPLACES the row rather than patching it:
// a caller must send back every field it wants to keep. Keeping the conversion
// in one place means a read-modify-write can never drop a field (or send the
// integer row id where the panel expects a uuid).
func SpecFromPanelClient(p *PanelClient) ClientSpec {
	if p == nil {
		return ClientSpec{}
	}
	return ClientSpec{
		Email:      p.Email,
		SubID:      p.SubID,
		UUID:       p.UUID,
		Password:   p.Password,
		Auth:       p.Auth,
		Flow:       p.Flow,
		Enable:     p.Enable,
		TotalGB:    p.TotalGB,
		ExpiryTime: p.ExpiryTime,
		LimitIP:    p.LimitIP,
		LimitHwid:  p.LimitHwid,
		Security:   p.Security,
		TgID:       p.TgID,
		Comment:    p.Comment,
	}
}

// AddClientRequest is the body of POST /panel/api/clients/add.
type AddClientRequest struct {
	Client     ClientSpec `json:"client"`
	InboundIDs []int      `json:"inboundIds"`
}
