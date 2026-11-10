// SPDX-FileCopyrightText: © 2026 OpenCHAMI a Series of LF Projects, LLC
//
// SPDX-License-Identifier: MIT

package main

import (
	"fmt"
	"log"
	"strings"

	"github.com/openchami/fabrica/pkg/events"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

// Event identity used before the feature loader owned event bus startup. The
// generated loader only sets the type prefix (defaulting to
// "metadata-service.resource") and leaves the source and condition prefix empty.
const (
	legacyEventTypePrefix = "io.fabrica"
	eventSource           = "metadata-service"
)

// setFlagDefault changes a generated flag's default without marking it as set,
// keeping --help accurate and letting config files override it.
func setFlagDefault(flags *pflag.FlagSet, name, value string) {
	flag := flags.Lookup(name)
	if flag == nil {
		panic(fmt.Errorf("cannot override default of unknown flag --%s", name))
	}
	if err := flag.Value.Set(value); err != nil {
		panic(fmt.Errorf("invalid default for --%s: %w", name, err))
	}
	flag.DefValue = value
}

// applyEventIdentityDefaults runs after loadGeneratedFeatures and restores the
// event identity the service emitted before adopting the loader, while still
// honoring an explicit event_type_prefix setting.
func applyEventIdentityDefaults() {
	config := events.GetEventConfig()
	if !viper.IsSet("event_type_prefix") {
		config.EventTypePrefix = legacyEventTypePrefix
	}
	if strings.TrimSpace(config.ConditionEventPrefix) == "" {
		config.ConditionEventPrefix = config.EventTypePrefix + ".condition"
	}
	if strings.TrimSpace(config.Source) == "" {
		config.Source = eventSource
	}
	events.SetEventConfig(config)
	log.Printf("Event identity: prefix=%s condition_prefix=%s source=%s",
		config.EventTypePrefix, config.ConditionEventPrefix, config.Source)
}
