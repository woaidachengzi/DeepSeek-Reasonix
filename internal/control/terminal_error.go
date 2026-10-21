package control

import (
	"errors"

	"reasonix/internal/i18n"
	"reasonix/internal/provider"
	"reasonix/internal/turnevent"
)

func explainLocalTerminalError(err error) error {
	// Storage failures can satisfy net.Error. Keep them out of transport recovery.
	if errors.Is(err, turnevent.ErrTurnLedgerUnavailable) {
		return err
	}
	if errors.Is(err, provider.ErrNoDeliverableOutput) {
		return &explainedError{msg: i18n.M.ProviderErrNoDeliverable, cause: err}
	}
	return nil
}
