package proxy

import (
	"sort"
	"time"
)

const (
	stateVersion     = 2
	DefaultAliasRole = "frontend"
)

type State struct {
	Version  int                `json:"version"`
	Services map[string]Service `json:"services"`
}

type Service struct {
	Name       string              `json:"name"`
	Alias      string              `json:"alias"`
	ActiveID   string              `json:"activeId,omitempty"`
	ActiveRole string              `json:"activeRole,omitempty"`
	AliasRole  string              `json:"aliasRole"`
	Instances  map[string]Instance `json:"instances"`
	CreatedAt  time.Time           `json:"createdAt"`
	UpdatedAt  time.Time           `json:"updatedAt"`

	LegacySwitchRole string `json:"switchRole,omitempty"`
}

type Instance struct {
	ID        string          `json:"id"`
	Roles     map[string]Role `json:"roles"`
	CreatedAt time.Time       `json:"createdAt"`
	UpdatedAt time.Time       `json:"updatedAt"`

	LegacyHost      string    `json:"host,omitempty"`
	LegacyPort      int       `json:"port,omitempty"`
	LegacyURL       string    `json:"url,omitempty"`
	LegacyCWD       string    `json:"cwd,omitempty"`
	LegacyCommand   []string  `json:"command,omitempty"`
	LegacyPID       int       `json:"pid,omitempty"`
	LegacyStartedAt time.Time `json:"startedAt,omitempty"`
}

type Role struct {
	Name       string               `json:"name"`
	Host       string               `json:"host"`
	Port       int                  `json:"port"`
	URL        string               `json:"url"`
	ExtraPorts map[string]ExtraPort `json:"extraPorts,omitempty"`
	CWD        string               `json:"cwd"`
	Command    []string             `json:"command"`
	PID        int                  `json:"pid"`
	StartedAt  time.Time            `json:"startedAt"`
	UpdatedAt  time.Time            `json:"updatedAt"`
}

type ExtraPort struct {
	Name string `json:"name"`
	Host string `json:"host"`
	Port int    `json:"port"`
	URL  string `json:"url"`
}

func NewState() State {
	return State{
		Version:  stateVersion,
		Services: make(map[string]Service),
	}
}

func (state State) SortedServices() []Service {
	services := make([]Service, 0, len(state.Services))
	for _, service := range state.Services {
		services = append(services, service)
	}
	sort.Slice(services, func(i int, j int) bool {
		return services[i].Name < services[j].Name
	})
	return services
}

func (state *State) Normalize() {
	if state.Version == 0 {
		state.Version = stateVersion
	}
	if state.Services == nil {
		state.Services = make(map[string]Service)
	}
	for serviceName, service := range state.Services {
		if service.Name == "" {
			service.Name = serviceName
		}
		if service.AliasRole == "" {
			service.AliasRole = service.LegacySwitchRole
		}
		if service.AliasRole == "" {
			service.AliasRole = DefaultAliasRole
		}
		if service.ActiveID != "" && service.ActiveRole == "" {
			service.ActiveRole = service.AliasRole
		}
		service.LegacySwitchRole = ""
		if service.Instances == nil {
			service.Instances = make(map[string]Instance)
		}
		for instanceID, instance := range service.Instances {
			if instance.ID == "" {
				instance.ID = instanceID
			}
			if instance.Roles == nil {
				instance.Roles = make(map[string]Role)
			}
			if instance.LegacyPort > 0 && len(instance.Roles) == 0 {
				roleName := service.AliasRole
				if roleName == "" {
					roleName = DefaultAliasRole
				}
				role := Role{
					Name:      roleName,
					Host:      instance.LegacyHost,
					Port:      instance.LegacyPort,
					URL:       instance.LegacyURL,
					CWD:       instance.LegacyCWD,
					Command:   append([]string(nil), instance.LegacyCommand...),
					PID:       instance.LegacyPID,
					StartedAt: instance.LegacyStartedAt,
					UpdatedAt: instance.UpdatedAt,
				}
				if role.Host == "" {
					role.Host = "127.0.0.1"
				}
				instance.Roles[roleName] = role
			}
			instance.LegacyHost = ""
			instance.LegacyPort = 0
			instance.LegacyURL = ""
			instance.LegacyCWD = ""
			instance.LegacyCommand = nil
			instance.LegacyPID = 0
			instance.LegacyStartedAt = time.Time{}
			service.Instances[instanceID] = instance
		}
		state.Services[serviceName] = service
	}
}

func (service Service) SortedInstances() []Instance {
	instances := make([]Instance, 0, len(service.Instances))
	for _, instance := range service.Instances {
		instances = append(instances, instance)
	}
	sort.Slice(instances, func(i int, j int) bool {
		return instances[i].ID < instances[j].ID
	})
	return instances
}

func (instance Instance) SortedRoles() []Role {
	roles := make([]Role, 0, len(instance.Roles))
	for _, role := range instance.Roles {
		roles = append(roles, role)
	}
	sort.Slice(roles, func(i int, j int) bool {
		return roles[i].Name < roles[j].Name
	})
	return roles
}
