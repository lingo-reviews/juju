// Copyright 2012-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package systemmanager_test

import (
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	commontesting "github.com/juju/juju/apiserver/common/testing"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/apiserver/systemmanager"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
)

// NOTE: the testing of the general environment destruction code
// is found in apiserver/common/environdestroy_test.go.
//
// The tests here are around the validation and behaviour of
// the flags passed in to the system manager destroy system call.

type destroySystemSuite struct {
	jujutesting.JujuConnSuite
	commontesting.BlockHelper

	systemManager *systemmanager.SystemManagerAPI

	otherState    *state.State
	otherEnvOwner names.UserTag
	otherEnvUUID  string
}

var _ = gc.Suite(&destroySystemSuite{})

func (s *destroySystemSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)

	s.BlockHelper = commontesting.NewBlockHelper(s.APIState)
	s.AddCleanup(func(*gc.C) { s.BlockHelper.Close() })

	resources := common.NewResources()
	s.AddCleanup(func(_ *gc.C) { resources.StopAll() })

	authoriser := apiservertesting.FakeAuthorizer{
		Tag: s.AdminUserTag(c),
	}
	systemManager, err := systemmanager.NewSystemManagerAPI(s.State, resources, authoriser)
	c.Assert(err, jc.ErrorIsNil)
	s.systemManager = systemManager

	s.otherEnvOwner = names.NewUserTag("jess@dummy")
	s.otherState = factory.NewFactory(s.State).MakeEnvironment(c, &factory.EnvParams{
		Name:    "dummytoo",
		Owner:   s.otherEnvOwner,
		Prepare: true,
		ConfigAttrs: testing.Attrs{
			"state-server": false,
		},
	})
	s.AddCleanup(func(c *gc.C) { s.otherState.Close() })
	s.otherEnvUUID = s.otherState.EnvironUUID()
}

func (s *destroySystemSuite) TestDestroySystemKillsHostedEnvsWithBlocks(c *gc.C) {
	s.startMockUndertaker(c)
	s.BlockDestroyEnvironment(c, "TestBlockDestroyEnvironment")
	s.BlockRemoveObject(c, "TestBlockRemoveObject")
	s.otherState.SwitchBlockOn(state.DestroyBlock, "TestBlockDestroyEnvironment")
	s.otherState.SwitchBlockOn(state.ChangeBlock, "TestChangeBlock")
	err := s.systemManager.DestroySystem(params.DestroySystemArgs{
		DestroyEnvironments: true,
		IgnoreBlocks:        true,
	})
	c.Assert(err, jc.ErrorIsNil)

	env, err := s.otherState.Environment()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(env.Life(), gc.Equals, state.Dead)

	env, err = s.State.Environment()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(env.Life(), gc.Equals, state.Dying)
}

func (s *destroySystemSuite) TestDestroySystemReturnsBlockedEnvironmentsErr(c *gc.C) {
	s.startMockUndertaker(c)
	s.BlockDestroyEnvironment(c, "TestBlockDestroyEnvironment")
	s.BlockRemoveObject(c, "TestBlockRemoveObject")
	s.otherState.SwitchBlockOn(state.DestroyBlock, "TestBlockDestroyEnvironment")
	s.otherState.SwitchBlockOn(state.ChangeBlock, "TestChangeBlock")

	err := s.systemManager.DestroySystem(params.DestroySystemArgs{
		DestroyEnvironments: true,
	})
	c.Assert(params.IsCodeOperationBlocked(err), jc.IsTrue)

	numBlocks, err := s.State.AllBlocksForSystem()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(numBlocks), gc.Equals, 4)

	_, err = s.otherState.Environment()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *destroySystemSuite) TestDestroySystemKillsHostedEnvs(c *gc.C) {
	s.startMockUndertaker(c)
	err := s.systemManager.DestroySystem(params.DestroySystemArgs{
		DestroyEnvironments: true,
	})
	c.Assert(err, jc.ErrorIsNil)

	env, err := s.otherState.Environment()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(env.Life(), gc.Equals, state.Dead)
	env, err = s.State.Environment()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(env.Life(), gc.Equals, state.Dying)
}

func (s *destroySystemSuite) TestDestroySystemLeavesBlocksIfNotKillAll(c *gc.C) {
	s.startMockUndertaker(c)
	s.BlockDestroyEnvironment(c, "TestBlockDestroyEnvironment")
	s.BlockRemoveObject(c, "TestBlockRemoveObject")
	s.otherState.SwitchBlockOn(state.DestroyBlock, "TestBlockDestroyEnvironment")
	s.otherState.SwitchBlockOn(state.ChangeBlock, "TestChangeBlock")

	err := s.systemManager.DestroySystem(params.DestroySystemArgs{
		IgnoreBlocks: true,
	})
	c.Assert(err, gc.ErrorMatches, "state server environment cannot be destroyed before all other environments are destroyed")

	numBlocks, err := s.State.AllBlocksForSystem()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(numBlocks), gc.Equals, 4)
}

func (s *destroySystemSuite) TestDestroySystemNoHostedEnvs(c *gc.C) {
	s.startMockUndertaker(c)
	err := common.DestroyEnvironment(s.State, s.otherState.EnvironTag())
	c.Assert(err, jc.ErrorIsNil)

	err = s.systemManager.DestroySystem(params.DestroySystemArgs{})
	c.Assert(err, jc.ErrorIsNil)

	env, err := s.State.Environment()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(env.Life(), gc.Equals, state.Dying)
}

func (s *destroySystemSuite) TestDestroySystemNoHostedEnvsWithBlock(c *gc.C) {
	s.startMockUndertaker(c)
	err := common.DestroyEnvironment(s.State, s.otherState.EnvironTag())
	c.Assert(err, jc.ErrorIsNil)

	s.BlockDestroyEnvironment(c, "TestBlockDestroyEnvironment")
	s.BlockRemoveObject(c, "TestBlockRemoveObject")

	err = s.systemManager.DestroySystem(params.DestroySystemArgs{
		IgnoreBlocks: true,
	})
	c.Assert(err, jc.ErrorIsNil)

	env, err := s.State.Environment()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(env.Life(), gc.Equals, state.Dying)
}

func (s *destroySystemSuite) TestDestroySystemNoHostedEnvsWithBlockFail(c *gc.C) {
	s.startMockUndertaker(c)
	err := common.DestroyEnvironment(s.State, s.otherState.EnvironTag())
	c.Assert(err, jc.ErrorIsNil)

	s.BlockDestroyEnvironment(c, "TestBlockDestroyEnvironment")
	s.BlockRemoveObject(c, "TestBlockRemoveObject")

	err = s.systemManager.DestroySystem(params.DestroySystemArgs{})
	c.Assert(params.IsCodeOperationBlocked(err), jc.IsTrue)

	numBlocks, err := s.State.AllBlocksForSystem()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(numBlocks), gc.Equals, 2)
}

// startMockUndertaker processes a dying environment and sets it to dead.
// Normally the undertaker worker, running on the machine agent, would
// process all dying environments. But for the unit tests, we run a mock
// undertaker in the background.
func (s *destroySystemSuite) startMockUndertaker(c *gc.C) {
	watcher := watchEnvironResources(c, s.otherState)
	s.AddCleanup(func(c *gc.C) { c.Assert(watcher.Stop(), jc.ErrorIsNil) })
	go func() {
		for {
			select {
			case _, ok := <-watcher.Changes():
				if !ok {
					return
				}
				if err := s.otherState.ProcessDyingEnviron(); err == nil {
					watcher.Stop()
					return
				}
			}
		}
	}()
}

func watchEnvironResources(c *gc.C, st *state.State) state.NotifyWatcher {
	env, err := st.Environment()
	c.Assert(err, jc.ErrorIsNil)
	machines, err := st.AllMachines()
	c.Assert(err, jc.ErrorIsNil)
	services, err := st.AllServices()
	c.Assert(err, jc.ErrorIsNil)

	watchers := []state.NotifyWatcher{env.Watch()}
	for _, machine := range machines {
		watchers = append(watchers, machine.Watch())
	}
	for _, service := range services {
		watchers = append(watchers, service.Watch())
	}
	return common.NewMultiNotifyWatcher(watchers...)
}
