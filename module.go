package geoyandex

import (
	"context"
	"errors"
	"net/http"

	"github.com/vernal96/go-cms-kernel"
	"github.com/vernal96/go-cms-kernel/modules/core"
	"github.com/vernal96/go-cms-kernel/modules/core/field"
	httptransport "github.com/vernal96/go-cms-kernel/transport/http"
)

const ModuleCode kernel.ModuleCode = "geo-yandex"

type Config struct {
	SuggestAPIKey      string
	GeocoderAPIKey     string
	JSAPIKey           string
	HTTPClient         *http.Client
	SuggestURL         string
	GeocoderURL        string
	RateLimitPerMinute int
	RateLimitKey       func(*http.Request) string
}

type module struct{ config Config }

func New(config Config) kernel.Module { return module{config: config} }

func (module) Code() kernel.ModuleCode           { return ModuleCode }
func (module) Dependencies() []kernel.ModuleCode { return []kernel.ModuleCode{core.ModuleCode} }
func (m module) Validate(ctx context.Context, _ kernel.ModuleValidationContext) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if m.config.RateLimitPerMinute < 0 {
		return errors.New("geo-yandex rate limit cannot be negative")
	}
	_, err := NewService(m.config)
	return err
}
func (m module) Build(ctx context.Context, _ kernel.ModuleContext) (kernel.ModuleRuntime, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if m.config.RateLimitPerMinute < 0 {
		return nil, errors.New("geo-yandex rate limit cannot be negative")
	}
	service, err := NewService(m.config)
	if err != nil {
		return nil, err
	}
	return &Runtime{service: service, jsAPIKey: m.config.JSAPIKey, rateLimitPerMinute: m.config.RateLimitPerMinute, rateLimitKey: m.config.RateLimitKey}, nil
}
func (module) DefinitionContributions() []kernel.DefinitionContribution {
	return []kernel.DefinitionContribution{kernel.DefineFor[*core.Definitions](core.ModuleCode, func(ctx context.Context, definitions *core.Definitions) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		return definitions.RegisterFieldType(field.DescribedType{
			Type: coordinateType{},
			Presentation: field.Metadata{
				Label:  "Координаты",
				Editor: CoordinateEditor,
			},
		})
	})}
}

type Runtime struct {
	service            *Service
	jsAPIKey           string
	rateLimitPerMinute int
	rateLimitKey       func(*http.Request) string
}

func (*Runtime) ModuleCode() kernel.ModuleCode { return ModuleCode }
func (r *Runtime) GeoYandex() *Service         { return r.service }
func (r *Runtime) HTTP() httptransport.Builder {
	return publicHTTP(r.service, r.jsAPIKey, r.rateLimitPerMinute, r.rateLimitKey)
}

var _ kernel.Module = module{}
var _ kernel.DependencyProvider = module{}
var _ kernel.DefinitionContributionProvider = module{}
var _ httptransport.Provider = (*Runtime)(nil)
