// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environment

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
	coretools "github.com/juju/juju/tools"
	"github.com/juju/juju/version"
)

var _ = gc.Suite(&updaterSuite{})

type updaterSuite struct {
	coretesting.BaseSuite
}

type dummyEnviron struct {
	environs.Environ
}

func (s *updaterSuite) TestCheckTools(c *gc.C) {
	sConfig := coretesting.FakeConfig()
	sConfig = sConfig.Merge(coretesting.Attrs{
		"agent-version": "2.5.0",
	})
	cfg, err := config.New(config.NoDefaults, sConfig)
	c.Assert(err, jc.ErrorIsNil)
	fakeNewEnvirons := func(*config.Config) (environs.Environ, error) {
		return dummyEnviron{}, nil
	}
	s.PatchValue(&newEnvirons, fakeNewEnvirons)
	var (
		calledWithEnviron                environs.Environ
		calledWithMajor, calledWithMinor int
		calledWithFilter                 coretools.Filter
	)
	fakeToolFinder := func(e environs.Environ, maj int, min int, _ string, filter coretools.Filter) (coretools.List, error) {
		calledWithEnviron = e
		calledWithMajor = maj
		calledWithMinor = min
		calledWithFilter = filter
		ver := version.Binary{Number: version.Number{Major: maj, Minor: min}}
		t := coretools.Tools{Version: ver, URL: "http://example.com", Size: 1}
		c.Assert(calledWithMajor, gc.Equals, 2)
		c.Assert(calledWithMinor, gc.Equals, 5)
		return coretools.List{&t}, nil
	}

	ver, err := checkToolsAvailability(cfg, fakeToolFinder)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ver, gc.Not(gc.Equals), version.Zero)
	c.Assert(ver, gc.Equals, version.Number{Major: 2, Minor: 5, Patch: 0})
}

type envGetter struct {
}

func (e *envGetter) Environment() (*state.Environment, error) {
	return &state.Environment{}, nil
}

func (s *updaterSuite) TestUpdateToolsAvailability(c *gc.C) {
	fakeNewEnvirons := func(*config.Config) (environs.Environ, error) {
		return dummyEnviron{}, nil
	}
	s.PatchValue(&newEnvirons, fakeNewEnvirons)

	fakeEnvConfig := func(_ *state.Environment) (*config.Config, error) {
		sConfig := coretesting.FakeConfig()
		sConfig = sConfig.Merge(coretesting.Attrs{
			"agent-version": "2.5.0",
		})
		return config.New(config.NoDefaults, sConfig)
	}
	s.PatchValue(&envConfig, fakeEnvConfig)

	fakeToolFinder := func(_ environs.Environ, _ int, _ int, _ string, _ coretools.Filter) (coretools.List, error) {
		ver := version.Binary{Number: version.Number{Major: 2, Minor: 5, Patch: 2}}
		olderVer := version.Binary{Number: version.Number{Major: 2, Minor: 5, Patch: 1}}
		t := coretools.Tools{Version: ver, URL: "http://example.com", Size: 1}
		tOld := coretools.Tools{Version: olderVer, URL: "http://example.com", Size: 1}
		return coretools.List{&t, &tOld}, nil
	}

	var ver version.Number
	fakeUpdate := func(_ *state.Environment, v version.Number) error {
		ver = v
		return nil
	}

	err := updateToolsAvailability(&envGetter{}, fakeToolFinder, fakeUpdate)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(ver, gc.Not(gc.Equals), version.Zero)
	c.Assert(ver, gc.Equals, version.Number{Major: 2, Minor: 5, Patch: 2})
}
