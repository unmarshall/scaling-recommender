package pricing

type instancePricing struct {
	InstanceType string       `json:"instance_type"`
	VCpu         float64      `json:"vcpu"`
	Memory       float64      `json:"memory"`
	EDPPrice     priceDetails `json:"edp_price"`
}

type priceDetails struct {
	PayAsYouGo    float64 `json:"pay_as_you_go"`
	Reserved1Year float64 `json:"ri_1_year"`
	Reserved3Year float64 `json:"ri_3_years"`
}

type allInstancePricing struct {
	Results []instancePricing `json:"results"`
}
