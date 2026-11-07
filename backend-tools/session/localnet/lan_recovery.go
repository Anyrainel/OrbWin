package localnet

import (
	"context"
	"errors"
	"time"

	"orbwin.local/session/internal/lanhelper"
	"orbwin.local/session/internal/lanlease"
)

// Recovery reads an already flushed protected operation acknowledgement. It
// must never infer Create/Delete completion from a matching rule name/presence.
type lanCreateAcknowledgementRecovery interface {
	RecoverCreate(context.Context, lanhelper.Prepared) (lanlease.RuleAcknowledgement, error)
}
type lanRemovalAcknowledgementRecovery interface {
	RecoverRemoval(context.Context, lanhelper.Prepared) (lanlease.RemovalAcknowledgement, error)
}

func (c *lanController) findSession(sha string) *lanSession {
	c.mu.Lock()
	defer c.mu.Unlock()
	s := c.sessions[sha]
	if s == nil {
		s = c.rules[sha]
	}
	return s
}
func (c *lanController) inspect(ctx context.Context, sha string) (LANStatus, error) {
	s := c.findSession(sha)
	if s == nil {
		return LANStatus{}, errors.New("exact retained LAN operation unavailable")
	}
	s.mu.Lock()
	unknownCreate, unknownRemove := s.unknownCommit, s.retirementDispatched && !s.lease.Snapshot(time.Now()).RuleRetired
	owned := s.ruleOwned && !s.lease.Snapshot(time.Now()).RuleRetired
	s.mu.Unlock()
	if !unknownCreate && !unknownRemove {
		if owned {
			current, err := c.adapters.Helper.ReadRule(ctx, s.prepared)
			if err == nil {
				s.mu.Lock()
				err = s.lease.CheckRetainedRule(current, time.Now())
				s.mu.Unlock()
			}
			if err != nil {
				c.closeSession(s)
				return c.sessionStatus(s), err
			}
		}
		return c.sessionStatus(s), nil
	}
	if unknownCreate {
		recovery, ok := c.adapters.Helper.(lanCreateAcknowledgementRecovery)
		if !ok {
			return c.sessionStatus(s), errors.New("protected LAN create recovery is unregistered")
		}
		ack, err := recovery.RecoverCreate(ctx, s.prepared)
		if err != nil {
			return c.sessionStatus(s), err
		}
		s.mu.Lock()
		err = s.lease.AcknowledgeRuleWhileClosing(s.lease.Snapshot(time.Now()).Revision, ack)
		if err == nil {
			s.ruleOwned = true
			s.unknownCommit = false
		}
		s.mu.Unlock()
		return c.sessionStatus(s), err
	}
	recovery, ok := c.adapters.Helper.(lanRemovalAcknowledgementRecovery)
	if !ok {
		return c.sessionStatus(s), errors.New("protected LAN removal recovery is unregistered")
	}
	ack, err := recovery.RecoverRemoval(ctx, s.prepared)
	if err != nil {
		return c.sessionStatus(s), err
	}
	s.mu.Lock()
	err = s.lease.AcknowledgeRemoved(s.lease.Snapshot(time.Now()).Revision, ack)
	s.mu.Unlock()
	return c.sessionStatus(s), err
}
func (c *lanController) settleHelper(ctx context.Context, sha string) (LANStatus, error) {
	s := c.findSession(sha)
	if s == nil {
		return LANStatus{}, errors.New("exact retained LAN helper unavailable")
	}
	s.mu.Lock()
	eligible := s.ingressClosed && s.borrowed
	handle := s.handle
	s.mu.Unlock()
	if !eligible {
		return c.sessionStatus(s), errors.New("helper settlement requires positive child/flow closure and retained exact reference")
	}
	if err := c.shared.settleAbsent(ctx, sha, handle); err != nil {
		return c.sessionStatus(s), err
	}
	s.mu.Lock()
	s.borrowed = false
	s.result = nil
	s.mu.Unlock()
	s.resourceOnce.Do(func() { close(s.closed) })
	c.mu.Lock()
	c.compactClosedLocked()
	c.mu.Unlock()
	return c.sessionStatus(s), nil
}
