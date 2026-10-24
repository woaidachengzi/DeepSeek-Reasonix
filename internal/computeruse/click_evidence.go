package computeruse

// ClickEvidence separates host input facts from application-level outcomes.
// Neither a posted mouse event nor a cursor readback acknowledges a DOM target.
type ClickEvidence struct {
	InputDelivery      string `json:"input_delivery"`
	TargetVerification string `json:"target_verification"`
	ApplicationEffect  string `json:"application_effect"`
}

func clickEvidence(action string, result Result) *ClickEvidence {
	if action != "click" && action != "confirm_click" {
		return nil
	}
	e := &ClickEvidence{InputDelivery: "not_sent", TargetVerification: "unverified", ApplicationEffect: "unverified"}
	if result.DispatchUnknown {
		e.InputDelivery = "unknown"
	} else if result.Dispatched {
		e.InputDelivery = "posted"
		if result.DispatchMethod == "window_server_mouse" {
			e.TargetVerification = "window_only"
		}
	}
	return e
}
