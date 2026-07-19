package workloads

import (
	"errors"
	"regexp"
	"sort"
	"strings"
)

type Kind string

const (
	KindApplication      Kind = "application"
	KindComposeStack     Kind = "compose-stack"
	KindStaticSite       Kind = "static-site"
	KindBackgroundWorker Kind = "background-worker"
	KindCronJob          Kind = "cron-job"
	KindGameServer       Kind = "game-server"
	KindDatabase         Kind = "database"
	KindCache            Kind = "cache"
	KindGenericContainer Kind = "generic-container"
	KindSystemContainer  Kind = "system-container"
	KindVirtualMachine   Kind = "virtual-machine"
)

var kindPattern = regexp.MustCompile(`^[a-z][a-z0-9]*(?:-[a-z0-9]+)*$`)

func (k Kind) Validate() error {
	value := strings.TrimSpace(string(k))
	if !kindPattern.MatchString(value) {
		return errors.New("workload kind must be a lowercase kebab-case identifier")
	}
	return nil
}

func UniqueKinds(kinds []Kind) ([]Kind, error) {
	seen := make(map[Kind]struct{}, len(kinds))
	result := make([]Kind, 0, len(kinds))
	for _, kind := range kinds {
		if err := kind.Validate(); err != nil {
			return nil, err
		}
		if _, exists := seen[kind]; exists {
			return nil, errors.New("duplicate workload kind: " + string(kind))
		}
		seen[kind] = struct{}{}
		result = append(result, kind)
	}
	sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })
	return result, nil
}
