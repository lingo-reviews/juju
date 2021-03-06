// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resolver

import (
	"github.com/juju/errors"
	"gopkg.in/juju/charm.v5"
	"gopkg.in/juju/charm.v5/hooks"
	"launchpad.net/tomb"

	"github.com/juju/juju/worker/charmdir"
	"github.com/juju/juju/worker/uniter/operation"
	"github.com/juju/juju/worker/uniter/remotestate"
)

// LoopConfig contains configuration parameters for the resolver loop.
type LoopConfig struct {
	Resolver       Resolver
	Watcher        remotestate.Watcher
	Executor       operation.Executor
	Factory        operation.Factory
	CharmURL       *charm.URL
	Conflicted     bool
	Dying          <-chan struct{}
	OnIdle         func() error
	CharmDirLocker charmdir.Locker
}

// Loop repeatedly waits for remote state changes, feeding the local and
// remote state to the provided Resolver to generate Operations which are
// then run with the provided Executor.
//
// The provided "onIdle" function will be called when the loop is waiting
// for remote state changes due to a lack of work to perform. It will not
// be called when a change is anticipated (i.e. due to ErrWaiting).
//
// The resolver loop can be controlled in the following ways:
//  - if the "dying" channel is signalled, then the loop will
//    exit with tomb.ErrDying
//  - if the resolver returns ErrWaiting, then no operations
//    will be executed until the remote state has changed
//    again
//  - if the resolver returns ErrNoOperation, then "onIdle"
//    will be invoked and the loop will wait until the remote
//    state has changed again
//  - if the resolver, onIdle, or executor return some other
//    error, the loop will exit immediately
//
// Loop will return the last LocalState acted upon, regardless of whether
// an error is returned. This can be used, for example, to obtain the
// charm URL being upgraded to.
func Loop(cfg LoopConfig) (LocalState, error) {
	rf := &resolverOpFactory{
		Factory: cfg.Factory,
		LocalState: LocalState{
			CharmURL:         cfg.CharmURL,
			Conflicted:       cfg.Conflicted,
			CompletedActions: map[string]struct{}{},
		},
	}

	// Initialize charmdir availability before entering the loop in case we're recovering from a restart.
	updateCharmDir(cfg.Executor.State(), cfg.CharmDirLocker)

	for {
		rf.RemoteState = cfg.Watcher.Snapshot()
		rf.LocalState.State = cfg.Executor.State()

		op, err := cfg.Resolver.NextOp(rf.LocalState, rf.RemoteState, rf)
		for err == nil {
			logger.Tracef("running op: %v", op)
			if err := cfg.Executor.Run(op); err != nil {
				return rf.LocalState, errors.Trace(err)
			}
			// Refresh snapshot, in case remote state
			// changed between operations.
			rf.RemoteState = cfg.Watcher.Snapshot()
			rf.LocalState.State = cfg.Executor.State()
			op, err = cfg.Resolver.NextOp(rf.LocalState, rf.RemoteState, rf)
		}

		updateCharmDir(rf.LocalState.State, cfg.CharmDirLocker)

		switch errors.Cause(err) {
		case nil:
		case ErrWaiting:
			// If a resolver is waiting for events to
			// complete, the agent is not idle.
		case ErrNoOperation:
			if cfg.OnIdle != nil {
				if err := cfg.OnIdle(); err != nil {
					return rf.LocalState, errors.Trace(err)
				}
			}
		default:
			return rf.LocalState, err
		}

		select {
		case <-cfg.Dying:
			return rf.LocalState, tomb.ErrDying
		case <-cfg.Watcher.RemoteStateChanged():
		}
	}
}

// updateCharmDir sets charm directory availability for sharing among
// concurrent workers according to local operation state.
func updateCharmDir(opState operation.State, locker charmdir.Locker) {
	var changing bool

	// Determine if the charm content is changing.
	if opState.Kind == operation.Install || opState.Kind == operation.Upgrade {
		changing = true
	} else if opState.Kind == operation.RunHook && opState.Hook != nil && opState.Hook.Kind == hooks.UpgradeCharm {
		changing = true
	}

	available := opState.Started && !opState.Stopped && !changing
	logger.Tracef("charmdir: available=%v opState: started=%v stopped=%v changing=%v",
		available, opState.Started, opState.Stopped, changing)
	locker.SetAvailable(available)
}
