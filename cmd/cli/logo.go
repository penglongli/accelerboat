// Copyright 2025 The AccelerBoat Authors.  All rights reserved.
// Use of this source code is governed by an Apache2
// license that can be found in the LICENSE file.

package main

import (
	"fmt"

	"github.com/pterm/pterm"
)

// Logo returns the full Long text for the root command: ASCII art logo + tagline + description.
func Logo() string {
	panel := pterm.DefaultHeader.WithMargin(8).
		WithBackgroundStyle(pterm.NewStyle(pterm.BgLightBlue)).
		WithTextStyle(pterm.NewStyle(pterm.FgLightWhite)).Sprint("Manage your AccelerBoat more easily.")
	pterm.Info.Prefix = pterm.Prefix{
		Text:  "Tips",
		Style: pterm.NewStyle(pterm.BgBlue, pterm.FgLightWhite),
	}
	return fmt.Sprintf(`
%s%s
`, panel, asciiLogo)
}

// Simple ASCII block style; line 1 only A and B tops so cceler/oat read as lowercase
var (
	asciiLogo = pterm.FgLightGreen.Sprint(`
    ___              _           ____             _   
   /   | __________ | | ___ _ __| __ )  ___   __ _| |_ 
  / /| |/ __/ __/ _ \ |/ _ \ '__|  _ \ / _ \ / _ | __|
 / ___ / /_/ /_/  __/ |  __/ |  | |_) | (_) | (_| | |_
/_/  |_\___\___\___|_|\___|__|  |____/ \___/ \__,_|\__|
`)
)
