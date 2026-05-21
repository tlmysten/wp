package proxy

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var envTemplatePattern = regexp.MustCompile(`\{\{([A-Za-z0-9_-]+)(?:\.([A-Za-z0-9_-]+))?\.(host|port|url)\}\}`)

func resolveEnvAssignments(store *Store, serviceName string, id string, currentRole Role, assignments []string) ([]string, error) {
	if len(assignments) == 0 {
		return nil, nil
	}

	state, err := store.Load()
	if err != nil {
		return nil, err
	}

	roles := make(map[string]Role)
	service, ok := state.Services[serviceName]
	if ok {
		instance, ok := service.Instances[id]
		if ok {
			for roleName, role := range instance.Roles {
				roles[roleName] = role
			}
		}
	}
	roles[currentRole.Name] = currentRole

	resolved := make([]string, 0, len(assignments))
	for _, assignment := range assignments {
		key, value, ok := strings.Cut(assignment, "=")
		if !ok || key == "" {
			return nil, fmt.Errorf("env assignment must be KEY=VALUE: %q", assignment)
		}
		rendered, err := renderEnvValue(value, roles)
		if err != nil {
			return nil, err
		}
		resolved = append(resolved, key+"="+rendered)
	}
	return resolved, nil
}

func renderEnvValue(value string, roles map[string]Role) (string, error) {
	var renderErr error
	rendered := envTemplatePattern.ReplaceAllStringFunc(value, func(match string) string {
		parts := envTemplatePattern.FindStringSubmatch(match)
		if len(parts) != 4 {
			renderErr = fmt.Errorf("invalid env template %q", match)
			return match
		}
		role, ok := roles[parts[1]]
		if !ok {
			renderErr = fmt.Errorf("unknown role %q in env template %q", parts[1], match)
			return match
		}
		if parts[2] != "" {
			extraPort, ok := role.ExtraPorts[parts[2]]
			if !ok {
				renderErr = fmt.Errorf("unknown extra port %q for role %q in env template %q", parts[2], parts[1], match)
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
			return role.Host
		case "port":
			return strconv.Itoa(role.Port)
		case "url":
			return role.URL
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
