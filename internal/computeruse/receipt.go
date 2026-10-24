package computeruse

import "time"

func expiredObservation(prior receipt, now time.Time) Result {
	return Result{BlockedReason: "observation_expired", ObservationTTLMS: ObservationTTL.Milliseconds(), ObservationAgeMS: max(0, now.Sub(prior.issued).Milliseconds()), NextAction: "observe"}
}
