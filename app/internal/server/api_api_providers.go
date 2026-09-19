package server

import (
	"github.com/gofiber/fiber/v2"
	"github.com/pausan/agenttik/app/internal/agent/apiprovider"
)

func (s *Server) apiProvider(c *fiber.Ctx) (*apiprovider.Provider, error) {
	provider, ok := s.registry.Get(c.Params("provider"))
	if !ok {
		return nil, badRequest("unknown API provider")
	}
	api, ok := provider.(*apiprovider.Provider)
	if !ok {
		return nil, badRequest("this provider uses a subscription login")
	}
	return api, nil
}
func (s *Server) configureAPIProvider(c *fiber.Ctx) error {
	provider, err := s.apiProvider(c)
	if err != nil {
		return err
	}
	var body struct {
		Key     string `json:"key"`
		Enabled bool   `json:"enabled"`
		Refresh bool   `json:"refresh"`
	}
	if c.BodyParser(&body) != nil {
		return badRequest("invalid API connection")
	}
	if err := provider.Configure(c.UserContext(), body.Key, body.Enabled, body.Refresh); err != nil {
		return badRequest("%v", err)
	}
	return s.listProviders(c)
}
func (s *Server) removeAPIProviderKey(c *fiber.Ctx) error {
	provider, err := s.apiProvider(c)
	if err != nil {
		return err
	}
	if err := provider.RemoveKey(); err != nil {
		return err
	}
	return s.listProviders(c)
}

func (s *Server) addAPIProvider(c *fiber.Ctx) error {
	// Profile-local listeners also publish connections to the instance.
	if s.apiRoot != nil {
		s.apiRoot.profiles.mu.RLock()
		defer s.apiRoot.profiles.mu.RUnlock()
		return s.apiRoot.addAPIProvider(c)
	}
	template, err := s.apiProvider(c)
	if err != nil {
		return err
	}
	var body struct {
		Name string `json:"name"`
		Key  string `json:"key"`
	}
	if c.BodyParser(&body) != nil {
		return badRequest("invalid API connection")
	}
	provider, err := template.Create(c.UserContext(), body.Name, body.Key)
	if err != nil {
		return badRequest("%v", err)
	}
	s.registry.Add(provider)
	if s.profiles != nil {
		for _, rt := range s.profiles.running {
			rt.server.registry.Add(provider.ForProfile(rt.server.store.Dir()))
		}
	}
	return s.listProviders(c)
}
