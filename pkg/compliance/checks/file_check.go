// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package checks

import (
	"errors"
	"fmt"
	"os"
	"os/user"
	"strconv"
	"syscall"

	"github.com/DataDog/datadog-agent/pkg/compliance"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var (
	// ErrPropertyKindNotSupported is returned for property kinds not supported by the check
	ErrPropertyKindNotSupported = errors.New("property kind not supported")

	// ErrPropertyNotSupported is returned for properties not supported by the check
	ErrPropertyNotSupported = errors.New("property not supported")
)

type fileCheck struct {
	baseCheck
	File *compliance.File
}

func (c *fileCheck) Run() error {
	// TODO: here we will introduce various cached results lookups

	log.Debugf("%s:%s file check: %s", c.framework, c.ruleID, c.File.Path)
	if c.File.Path != "" {
		return c.reportFile(c.File.Path)
	}

	return log.Error("no path for file check")
}

func (c *fileCheck) reportFile(filePath string) error {
	kv := compliance.KV{}
	var v string

	fi, err := os.Stat(filePath)
	if err != nil {
		return log.Errorf("failed to stat %s", filePath)
	}

	for _, field := range c.File.Report {

		key := field.As

		if field.Value != "" {
			if key == "" {
				// TODO: error here
				continue
			}

			kv[key] = field.Value
			continue
		}

		switch field.Kind {
		case compliance.PropertyKindAttribute:
			v, err = c.getAttribute(fi, field.Property)
		case compliance.PropertyKindJSONPath:
			v, err = c.getJSONPathValue(filePath, field.Property)
		default:
			return ErrPropertyKindNotSupported
		}

		if key == "" {
			key = field.Property
		}

		kv[key] = v
	}
	c.report(nil, kv)
	return nil
}

func (c *fileCheck) getAttribute(fi os.FileInfo, property string) (string, error) {
	switch property {
	case "permissions":
		return fmt.Sprintf("%3o", fi.Mode()&os.ModePerm), nil
	case "owner":
		if statt, ok := fi.Sys().(*syscall.Stat_t); ok {
			var (
				u = strconv.Itoa(int(statt.Gid))
				g = strconv.Itoa(int(statt.Uid))
			)
			if group, err := user.LookupGroupId(g); err == nil {
				g = group.Name
			}
			if user, err := user.LookupId(u); err == nil {
				u = user.Username
			}
			return fmt.Sprintf("%s:%s", u, g), nil
		}
	}
	return "", ErrPropertyNotSupported
}

func (c *fileCheck) getJSONPathValue(filePath string, jsonPath string) (string, error) {
	// TODO: implement
	return "", nil
}