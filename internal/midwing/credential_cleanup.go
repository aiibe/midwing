package midwing

import (
	"errors"
)

// ChangeResult reports a committed change even when credential cleanup needs retrying.
type ChangeResult struct {
	Warning string `json:"warning,omitempty"`
}

type CommittedChangeError struct{}

func (*CommittedChangeError) Error() string {
	return "change saved, but credential cleanup is pending; it will retry on the next connection save, disconnect, or service deletion"
}

// Queue cleanup in the same atomic write that removes the connection. A failed
// settings write leaves credentials intact; a vault failure leaves a retry record.
func removeConnections(cfg *Settings, service string) {
	remaining := []Connection{}
	for _, conn := range cfg.Connections {
		if conn.Service == service {
			cfg.PendingCredentials = append(cfg.PendingCredentials, conn.CredentialRef)
		} else {
			remaining = append(remaining, conn)
		}
	}
	cfg.Connections = remaining
}

// Runs under the settings lock after committing the user-visible change.
func (s *Store) finishCredentialCleanup(cfg Settings) error {
	if len(cfg.PendingCredentials) == 0 {
		return nil
	}
	for _, ref := range cfg.PendingCredentials {
		// Never delete a credential that an edited settings file made active again.
		for _, conn := range cfg.Connections {
			if conn.CredentialRef == ref {
				return &CommittedChangeError{}
			}
		}
		if err := s.Vault.Delete(ref); err != nil && !errors.Is(err, errCredentialNotFound) {
			return &CommittedChangeError{}
		}
	}
	cfg.PendingCredentials = nil
	if err := s.write(cfg); err != nil {
		return &CommittedChangeError{}
	}
	return nil
}
