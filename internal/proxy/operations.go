package proxy

import (
	"context"
	"fmt"
	"time"
)

func UpsertService(ctx context.Context, store *Store, name string, alias string, switchRole string) (Service, error) {
	if name == "" {
		return Service{}, fmt.Errorf("service name is required")
	}
	if alias == "" {
		return Service{}, fmt.Errorf("service alias is required")
	}
	if switchRole == "" {
		switchRole = DefaultSwitchRole
	}

	var saved Service
	err := store.Update(ctx, func(state *State) error {
		now := time.Now()
		service := state.Services[name]
		if service.Name == "" {
			service = Service{
				Name:      name,
				Instances: make(map[string]Instance),
				CreatedAt: now,
			}
		}
		if service.Instances == nil {
			service.Instances = make(map[string]Instance)
		}
		service.Alias = alias
		service.SwitchRole = switchRole
		service.UpdatedAt = now
		state.Services[name] = service
		saved = service
		return nil
	})
	return saved, err
}

func RemoveService(ctx context.Context, store *Store, name string) error {
	if name == "" {
		return fmt.Errorf("service name is required")
	}
	return store.Update(ctx, func(state *State) error {
		if _, ok := state.Services[name]; !ok {
			return fmt.Errorf("unknown service %q", name)
		}
		delete(state.Services, name)
		return nil
	})
}

func RegisterRole(ctx context.Context, store *Store, serviceName string, id string, role Role) error {
	if serviceName == "" {
		return fmt.Errorf("service name is required")
	}
	if id == "" {
		return fmt.Errorf("instance id is required")
	}
	if role.Name == "" {
		return fmt.Errorf("role name is required")
	}
	if role.Port <= 0 {
		return fmt.Errorf("role port is required")
	}
	return store.Update(ctx, func(state *State) error {
		service, ok := state.Services[serviceName]
		if !ok {
			return fmt.Errorf("unknown service %q; add it with `wp proxy service add %s --alias <domain>`", serviceName, serviceName)
		}
		if service.Instances == nil {
			service.Instances = make(map[string]Instance)
		}
		instance := service.Instances[id]
		if instance.ID == "" {
			instance = Instance{
				ID:        id,
				Roles:     make(map[string]Role),
				CreatedAt: time.Now(),
			}
		}
		if instance.Roles == nil {
			instance.Roles = make(map[string]Role)
		}
		if _, ok := instance.Roles[role.Name]; ok {
			return fmt.Errorf("role %q is already registered for service %q instance %q", role.Name, serviceName, id)
		}
		now := time.Now()
		role.UpdatedAt = now
		if role.StartedAt.IsZero() {
			role.StartedAt = now
		}
		instance.Roles[role.Name] = role
		instance.UpdatedAt = now
		service.Instances[id] = instance
		service.UpdatedAt = now
		state.Services[serviceName] = service
		return nil
	})
}

func UnregisterInstance(ctx context.Context, store *Store, serviceName string, id string) error {
	if serviceName == "" {
		return fmt.Errorf("service name is required")
	}
	if id == "" {
		return fmt.Errorf("instance id is required")
	}
	return store.Update(ctx, func(state *State) error {
		service, ok := state.Services[serviceName]
		if !ok {
			return fmt.Errorf("unknown service %q", serviceName)
		}
		if _, ok := service.Instances[id]; !ok {
			return fmt.Errorf("unknown instance %q for service %q", id, serviceName)
		}
		delete(service.Instances, id)
		if service.ActiveID == id {
			service.ActiveID = ""
		}
		service.UpdatedAt = time.Now()
		state.Services[serviceName] = service
		return nil
	})
}

func UnregisterRole(ctx context.Context, store *Store, serviceName string, id string, roleName string) error {
	if serviceName == "" {
		return fmt.Errorf("service name is required")
	}
	if id == "" {
		return fmt.Errorf("instance id is required")
	}
	if roleName == "" {
		return fmt.Errorf("role name is required")
	}
	return store.Update(ctx, func(state *State) error {
		service, ok := state.Services[serviceName]
		if !ok {
			return fmt.Errorf("unknown service %q", serviceName)
		}
		instance, ok := service.Instances[id]
		if !ok {
			return fmt.Errorf("unknown instance %q for service %q", id, serviceName)
		}
		if _, ok := instance.Roles[roleName]; !ok {
			return fmt.Errorf("unknown role %q for service %q instance %q", roleName, serviceName, id)
		}
		delete(instance.Roles, roleName)
		if len(instance.Roles) == 0 {
			delete(service.Instances, id)
			if service.ActiveID == id {
				service.ActiveID = ""
			}
		} else {
			instance.UpdatedAt = time.Now()
			service.Instances[id] = instance
			if service.ActiveID == id && service.SwitchRole == roleName {
				service.ActiveID = ""
			}
		}
		service.UpdatedAt = time.Now()
		state.Services[serviceName] = service
		return nil
	})
}

func SwitchInstance(ctx context.Context, store *Store, backend Backend, serviceName string, id string) (Service, Instance, Role, error) {
	if serviceName == "" {
		return Service{}, Instance{}, Role{}, fmt.Errorf("service name is required")
	}
	if id == "" {
		return Service{}, Instance{}, Role{}, fmt.Errorf("instance id is required")
	}

	var switchedService Service
	var switchedInstance Instance
	var switchedRole Role
	err := store.Update(ctx, func(state *State) error {
		service, ok := state.Services[serviceName]
		if !ok {
			return fmt.Errorf("unknown service %q", serviceName)
		}
		instance, ok := service.Instances[id]
		if !ok {
			return fmt.Errorf("unknown instance %q for service %q", id, serviceName)
		}
		role, ok := instance.Roles[service.SwitchRole]
		if !ok {
			return fmt.Errorf("unknown switch role %q for service %q instance %q", service.SwitchRole, serviceName, id)
		}
		if backend == nil {
			return fmt.Errorf("proxy backend is required")
		}
		if err := backend.Apply(ctx, service, role); err != nil {
			return err
		}
		service.ActiveID = id
		service.UpdatedAt = time.Now()
		state.Services[serviceName] = service
		switchedService = service
		switchedInstance = instance
		switchedRole = role
		return nil
	})
	return switchedService, switchedInstance, switchedRole, err
}
