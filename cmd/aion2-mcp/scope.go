package main

import aion2 "github.com/nuriland/aion2-api"

// scope picks the client a call goes to, required by every tool.
type scope struct {
	Region aion2.Region `json:"region,omitempty"`
	Locale aion2.Locale `json:"locale,omitempty"`
}

func (s scope) target() scope { return s }

type scoped interface{ target() scope }
