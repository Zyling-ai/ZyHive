package skillopt

// IsRejected reports whether a proposal fingerprint is in the rejection buffer.
func (e EpochState) IsRejected(fingerprint string) bool {
	for _, fp := range e.RejectionBuffer {
		if fp == fingerprint {
			return true
		}
	}
	return false
}

// Reject moves a proposal into the rejection buffer (dedup by fingerprint) and
// marks it rejected. The epoch's shadow fields are cleared by the caller
// (shadow.Rollback) when the rejection follows a failed canary.
func Reject(s *Store, p Proposal) error {
	ep, err := s.ReadEpoch()
	if err != nil {
		return err
	}
	if p.Fingerprint != "" && !ep.IsRejected(p.Fingerprint) {
		ep.RejectionBuffer = append(ep.RejectionBuffer, p.Fingerprint)
	}
	if err := s.WriteEpoch(ep); err != nil {
		return err
	}
	p.Status = StatusRejected
	return s.WriteProposal(p)
}
