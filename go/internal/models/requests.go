package models

// APIRequest is the common request structure for all API endpoints.
// PHP API routes via the "actions" field in JSON body.
type APIRequest struct {
	Actions string `json:"actions"`
}

// APIResponse is the standard response format matching PHP's sendJsonResponse().
type APIResponse struct {
	Status bool        `json:"status"`
	Msg    string      `json:"msg"`
	Obj    interface{} `json:"obj"`
}

// PaginatedResponse wraps list results with pagination info.
type PaginatedResponse struct {
	Data       interface{} `json:"data"`
	Total      int64       `json:"total"`
	Page       int         `json:"page"`
	Limit      int         `json:"limit"`
	TotalPages int         `json:"total_pages"`
}

// --- User API Request Payloads ---

type UsersListRequest struct {
	Actions string `json:"actions"`
	Limit   int    `json:"limit,omitempty"`
	Page    int    `json:"page,omitempty"`
	Q       string `json:"q,omitempty"`
	Agent   string `json:"agent,omitempty"`
}

type UserDetailRequest struct {
	Actions string `json:"actions"`
	ChatID  string `json:"chat_id"`
}

type UserAddRequest struct {
	Actions string `json:"actions"`
	ChatID  string `json:"chat_id"`
}

type BlockUserRequest struct {
	Actions     string `json:"actions"`
	ChatID      string `json:"chat_id"`
	Description string `json:"description"`
	TypeBlock   string `json:"type_block"` // "block" or "unblock"
}

type VerifyUserRequest struct {
	Actions    string `json:"actions"`
	ChatID     string `json:"chat_id"`
	TypeVerify string `json:"type_verify"` // "1" or "0"
}

type ChangeStatusUserRequest struct {
	Actions string `json:"actions"`
	ChatID  string `json:"chat_id"`
	Type    string `json:"type"` // "active" or "inactive"
}

type BalanceRequest struct {
	Actions string `json:"actions"`
	ChatID  string `json:"chat_id"`
	Amount  int    `json:"amount"`
}

type SendMessageRequest struct {
	Actions     string `json:"actions"`
	ChatID      string `json:"chat_id"`
	Text        string `json:"text"`
	File        string `json:"file,omitempty"`
	ContentType string `json:"content_type,omitempty"`
}

type SetLimitTestRequest struct {
	Actions   string `json:"actions"`
	ChatID    string `json:"chat_id"`
	LimitTest int    `json:"limit_test"`
}

type TransferAccountRequest struct {
	Actions   string `json:"actions"`
	ChatID    string `json:"chat_id"`
	NewUserID string `json:"new_userid"`
}

type SetAgentRequest struct {
	Actions   string `json:"actions"`
	ChatID    string `json:"chat_id"`
	AgentType string `json:"agent_type,omitempty"`
}

type SetExpireAgentRequest struct {
	Actions    string `json:"actions"`
	ChatID     string `json:"chat_id"`
	ExpireTime int    `json:"expire_time"`
}

type SetBecomingNegativeRequest struct {
	Actions string `json:"actions"`
	ChatID  string `json:"chat_id"`
	Amount  int    `json:"amount"`
}

type SetPercentageDiscountRequest struct {
	Actions    string `json:"actions"`
	ChatID     string `json:"chat_id"`
	Percentage int    `json:"percentage"`
}

type ActiveBotAgentRequest struct {
	Actions string `json:"actions"`
	ChatID  string `json:"chat_id"`
	Token   string `json:"token"`
}

type CronNotifRequest struct {
	Actions string `json:"actions"`
	ChatID  string `json:"chat_id"`
	Type    string `json:"type"` // "1" or "0"
}

type SetPanelAgentShowRequest struct {
	Actions string      `json:"actions"`
	ChatID  string      `json:"chat_id"`
	Panels  interface{} `json:"panels"`
}

type SetLimitChangeLocationRequest struct {
	Actions string `json:"actions"`
	ChatID  string `json:"chat_id"`
	Limit   int    `json:"Limit"`
}

type SetPriceAgentBotRequest struct {
	Actions string `json:"actions"`
	ChatID  string `json:"chat_id"`
	Amount  int    `json:"amount"`
}

// --- Product API Request Payloads ---

type ProductsListRequest struct {
	Actions string `json:"actions"`
	Limit   int    `json:"limit,omitempty"`
	Page    int    `json:"page,omitempty"`
	Q       string `json:"q,omitempty"`
}

type ProductDetailRequest struct {
	Actions string `json:"actions"`
	ID      int    `json:"id"`
}

type ProductAddRequest struct {
	Actions        string      `json:"actions"`
	Name           string      `json:"name"`
	Price          int         `json:"price"`
	DataLimit      int         `json:"data_limit"`
	Time           int         `json:"time"`
	Location       string      `json:"location"`
	Agent          string      `json:"agent,omitempty"`
	Note           string      `json:"note,omitempty"`
	DataLimitReset string      `json:"data_limit_reset,omitempty"`
	Inbounds       interface{} `json:"inbounds,omitempty"`
	Proxies        interface{} `json:"proxies,omitempty"`
	Category       string      `json:"category,omitempty"`
	OneBuyStatus   int         `json:"one_buy_status,omitempty"`
	HidePanel      interface{} `json:"hide_panel,omitempty"`
}

type ProductEditRequest struct {
	Actions        string      `json:"actions"`
	ID             int         `json:"id"`
	Name           string      `json:"name,omitempty"`
	Price          int         `json:"price,omitempty"`
	Volume         int         `json:"volume,omitempty"`
	Time           int         `json:"time,omitempty"`
	Location       string      `json:"location,omitempty"`
	Agent          string      `json:"agent,omitempty"`
	Note           string      `json:"note,omitempty"`
	DataLimitReset string      `json:"data_limit_reset,omitempty"`
	Inbounds       interface{} `json:"inbounds,omitempty"`
	Proxies        interface{} `json:"proxies,omitempty"`
	Category       string      `json:"category,omitempty"`
	OneBuyStatus   int         `json:"one_buy_status,omitempty"`
	HidePanel      interface{} `json:"hide_panel,omitempty"`
}

type ProductDeleteRequest struct {
	Actions string `json:"actions"`
	ID      int    `json:"id"`
}

type SetInboundsRequest struct {
	Actions string `json:"actions"`
	ID      int    `json:"id"`
	Input   string `json:"input"`
}

// --- Invoice API Request Payloads ---

type InvoicesListRequest struct {
	Actions string `json:"actions"`
	Limit   int    `json:"limit,omitempty"`
	Page    int    `json:"page,omitempty"`
	Q       string `json:"q,omitempty"`
}

type InvoiceDetailRequest struct {
	Actions   string `json:"actions"`
	IDInvoice string `json:"id_invoice"`
}

type InvoiceAddRequest struct {
	Actions       string `json:"actions"`
	ChatID        string `json:"chat_id"`
	Username      string `json:"username"`
	CodeProduct   string `json:"code_product"`
	LocationCode  string `json:"location_code"`
	TimeService   int    `json:"time_service,omitempty"`
	VolumeService int    `json:"volume_service,omitempty"`
}

type RemoveServiceRequest struct {
	Actions   string `json:"actions"`
	IDInvoice string `json:"id_invoice"`
	Type      string `json:"type"` // "one", "tow", "three"
	Amount    int    `json:"amount,omitempty"`
}

type ChangeStatusConfigRequest struct {
	Actions   string `json:"actions"`
	IDInvoice string `json:"id_invoice"`
}

type ExtendServiceAdminRequest struct {
	Actions       string `json:"actions"`
	IDInvoice     string `json:"id_invoice"`
	TimeService   int    `json:"time_service"`
	VolumeService int    `json:"volume_service"`
}

// --- Payment API Request Payloads ---

type PaymentsListRequest struct {
	Actions string `json:"actions"`
	Limit   int    `json:"limit,omitempty"`
	Page    int    `json:"page,omitempty"`
	Q       string `json:"q,omitempty"`
}

type PaymentDetailRequest struct {
	Actions string `json:"actions"`
	IDOrder string `json:"id_order"`
}

// --- Panel API Request Payloads ---

type PanelsListRequest struct {
	Actions string `json:"actions"`
	Limit   int    `json:"limit,omitempty"`
	Page    int    `json:"page,omitempty"`
	Q       string `json:"q,omitempty"`
}

type PanelDetailRequest struct {
	Actions string `json:"actions"`
	ID      int    `json:"id"`
}

type PanelAddRequest struct {
	Actions        string      `json:"actions"`
	Name           string      `json:"name"`
	Price          int         `json:"price"`
	DataLimit      int         `json:"data_limit"`
	Time           int         `json:"time"`
	Location       string      `json:"location"`
	Agent          string      `json:"agent,omitempty"`
	Note           string      `json:"note,omitempty"`
	DataLimitReset string      `json:"data_limit_reset,omitempty"`
	Inbounds       interface{} `json:"inbounds,omitempty"`
	Proxies        interface{} `json:"proxies,omitempty"`
	Category       string      `json:"category,omitempty"`
	OneBuyStatus   int         `json:"one_buy_status,omitempty"`
	HidePanel      interface{} `json:"hide_panel,omitempty"`
}

type PanelEditRequest struct {
	Actions        string      `json:"actions"`
	ID             int         `json:"id"`
	Name           string      `json:"name,omitempty"`
	SubLink        string      `json:"sublink,omitempty"`
	Config         string      `json:"config,omitempty"`
	Status         string      `json:"status,omitempty"`
	Location       string      `json:"location,omitempty"`
	Agent          string      `json:"agent,omitempty"`
	Note           string      `json:"note,omitempty"`
	DataLimitReset string      `json:"data_limit_reset,omitempty"`
	Inbounds       interface{} `json:"inbounds,omitempty"`
	Proxies        interface{} `json:"proxies,omitempty"`
	Category       string      `json:"category,omitempty"`
	OneBuyStatus   int         `json:"one_buy_status,omitempty"`
	HidePanel      interface{} `json:"hide_panel,omitempty"`
}

type PanelDeleteRequest struct {
	Actions string `json:"actions"`
	ID      int    `json:"id"`
}

// --- Discount API Request Payloads ---

type DiscountsListRequest struct {
	Actions string `json:"actions"`
	Limit   int    `json:"limit,omitempty"`
	Page    int    `json:"page,omitempty"`
	Q       string `json:"q,omitempty"`
}

type DiscountDetailRequest struct {
	Actions string `json:"actions"`
	ID      int    `json:"id"`
}

type DiscountAddRequest struct {
	Actions  string `json:"actions"`
	Code     string `json:"code"`
	Price    int    `json:"price"`
	LimitUse int    `json:"limit_use"`
}

type DiscountDeleteRequest struct {
	Actions string `json:"actions"`
	ID      int    `json:"id"`
}

type DiscountSellAddRequest struct {
	Actions     string `json:"actions"`
	Code        string `json:"code"`
	Percent     int    `json:"percent"`
	LimitUse    int    `json:"limit_use"`
	Agent       string `json:"agent,omitempty"`
	UseFirst    string `json:"usefirst,omitempty"`
	UseUser     string `json:"useuser,omitempty"`
	CodeProduct string `json:"code_product,omitempty"`
	CodePanel   string `json:"code_panel,omitempty"`
	Time        string `json:"time,omitempty"`
	Type        string `json:"type,omitempty"`
}

type DiscountSellDeleteRequest struct {
	Actions string `json:"actions"`
	ID      int    `json:"id"`
}

// --- Category API Request Payloads ---

type CategoriesListRequest struct {
	Actions string `json:"actions"`
	Limit   int    `json:"limit,omitempty"`
	Page    int    `json:"page,omitempty"`
	Q       string `json:"q,omitempty"`
}

type CategoryDetailRequest struct {
	Actions string `json:"actions"`
	ID      int    `json:"id"`
}

type CategoryAddRequest struct {
	Actions string `json:"actions"`
	Remark  string `json:"remark"`
}

type CategoryEditRequest struct {
	Actions string `json:"actions"`
	ID      int    `json:"id"`
	Remark  string `json:"remark,omitempty"`
}

type CategoryDeleteRequest struct {
	Actions string `json:"actions"`
	ID      int    `json:"id"`
}

// --- Settings API Request Payloads ---

type KeyboardSetRequest struct {
	Actions       string      `json:"actions"`
	Keyboard      interface{} `json:"keyboard,omitempty"`
	KeyboardReset bool        `json:"keyboard_reset,omitempty"`
}

type SettingInfoRequest struct {
	Actions string `json:"actions"`
}

type SaveSettingShopRequest struct {
	Actions string            `json:"actions"`
	Data    []SettingShopItem `json:"data"`
}

type SettingShopItem struct {
	NameValue string      `json:"name_value"`
	Value     interface{} `json:"value"`
	Type      string      `json:"type"` // "shop" or "general"
	JSON      bool        `json:"json,omitempty"`
}
