package viampets

import (
	"context"
	"fmt"
	"time"

	"go.viam.com/rdk/components/generic"
	"go.viam.com/rdk/components/motor"
	"go.viam.com/rdk/logging"
	"go.viam.com/rdk/resource"
	"go.viam.com/utils"
)

var FeederModel = resource.ModelNamespace("erh").WithFamily("viampets").WithModel("feeder")

func init() {
	resource.RegisterComponent(
		generic.API,
		FeederModel,
		resource.Registration[resource.Resource, *feederConfig]{
			Constructor: newFeeder,
		})
}

type feederConfig struct {
	Motor  string
	Camera string

	SecondsToFeed float64 `json:"seconds_to_feed"`
}

func (cfg feederConfig) Validate(path string) ([]string, error) {
	deps := []string{cfg.Motor, cfg.Camera}

	if cfg.Motor == "" {
		return nil, utils.NewConfigValidationFieldRequiredError(path, "motor")
	}

	if cfg.Camera == "" {
		return nil, utils.NewConfigValidationFieldRequiredError(path, "camera")
	}

	return deps, nil
}

type feeder struct {
	resource.AlwaysRebuild

	config *feederConfig
	name   resource.Name
	logger logging.Logger

	backgroundContext context.Context
	backgroundCancel  context.CancelFunc

	theMotor motor.Motor
}

func newFeeder(ctx context.Context, deps resource.Dependencies, config resource.Config, logger logging.Logger) (resource.Resource, error) {
	newConf, err := resource.NativeConfig[*feederConfig](config)
	if err != nil {
		return nil, err
	}

	f := &feeder{config: newConf, name: config.ResourceName(), logger: logger}

	m, err := deps.Lookup(motor.Named(f.config.Motor))
	if err != nil {
		return nil, err
	}

	f.theMotor = m.(motor.Motor)

	return f, nil
}

func (f *feeder) Name() resource.Name {
	return f.name
}

func (f *feeder) run() {
	f.backgroundContext, f.backgroundCancel = context.WithCancel(context.Background())
}

func (f *feeder) feed(ctx context.Context) error {
	err := f.theMotor.SetPower(ctx, .5, nil)
	if err != nil {
		return err
	}
	if f.config.SecondsToFeed <= 0 {
		f.config.SecondsToFeed = 3
	}
	time.Sleep(time.Duration(float64(time.Second) * f.config.SecondsToFeed))
	return f.theMotor.Stop(ctx, nil)
}

func (f *feeder) DoCommand(ctx context.Context, cmd map[string]interface{}) (map[string]interface{}, error) {
	cmdName := cmd["cmd"]

	if cmdName == "feed" {
		return map[string]interface{}{}, f.feed(ctx)
	}

	return nil, fmt.Errorf("feeder unknown command [%s]", cmdName)
}

func (f *feeder) Close(ctx context.Context) error {
	f.backgroundCancel()
	return f.theMotor.Stop(ctx, nil)
}
