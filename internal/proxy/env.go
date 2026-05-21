package proxy

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var envTemplatePattern = regexp.MustCompile(`\{\{([A-Za-z0-9_-]+)(?:\.([A-Za-z0-9_-]+))?\.(host|port|url)\}\}`)

func resolveEnvAssignments(store *Store, currentServiceName string, id string, currentInstance Instance, assignments []string) ([]string, error) {
	if len(assignments) == 0 {
		return nil, nil
	}

	state, err := store.Load()
	if err != nil {
		return nil, err
	}

	instances := make(map[string]Instance)
	for _, service := range state.Services {
		if service.Name == currentServiceName {
			instances[service.Name] = currentInstance
			continue
		}
		if instance, ok := service.Instances[id]; ok {
			instances[service.Name] = instance
			continue
		}
		if service.ActiveID != "" {
			if instance, ok := service.Instances[service.ActiveID]; ok {
				instances[service.Name] = instance
			}
		}
	}
	instances[currentServiceName] = currentInstance

	resolved := make([]string, 0, len(assignments))
	for _, assignment := range assignments {
		key, value, ok := strings.Cut(assignment, "=")
		if !ok || key == "" {
			return nil, fmt.Errorf("env assignment must be KEY=VALUE: %q", assignment)
		}
		rendered, err := renderEnvValue(value, instances)
		if err != nil {
			return nil, err
		}
		resolved = append(resolved, key+"="+rendered)
	}
	return resolved, nil
}

func renderEnvValue(value string, instances map[string]Instance) (string, error) {
	var renderErr error
	rendered := envTemplatePattern.ReplaceAllStringFunc(value, func(match string) string {
		parts := envTemplatePattern.FindStringSubmatch(match)
		if len(parts) != 4 {
			renderErr = fmt.Errorf("invalid env template %q", match)
			return match
		}
		serviceName := parts[1]
		instance, ok := instances[serviceName]
		if !ok {
			renderErr = fmt.Errorf("unknown service %q in env template %q", serviceName, match)
			return match
		}
		if parts[2] != "" {
			extraPort, ok := instance.ExtraPorts[parts[2]]
			if !ok {
				renderErr = fmt.Errorf("unknown extra port %q for service %q in env template %q", parts[2], serviceName, match)
				return match
			}
			switch parts[3] {
			case "host":
				return extraPort.Host
			case "port":
				return strconv.Itoa(extraPort.Port)
			case "url":
				return extraPort.URL
			default:
				renderErr = fmt.Errorf("unknown field %q in env template %q", parts[3], match)
				return match
			}
		}
		switch parts[3] {
		case "host":
			return instance.Host
		case "port":
			return strconv.Itoa(instance.Port)
		case "url":
			return instance.URL
		default:
			renderErr = fmt.Errorf("unknown field %q in env template %q", parts[3], match)
			return match
		}
	})
	if renderErr != nil {
		return "", renderErr
	}
	if strings.Contains(rendered, "{{") || strings.Contains(rendered, "}}") {
		return "", fmt.Errorf("invalid env template in %q", value)
	}
	return rendered, nil
}
