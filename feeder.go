package viampets

import (
	"context"
	"fmt"
	"time"

	"go.viam.com/rdk/components/generic"
	"go.viam.com/rdk/components/motor"
	"go.viam.com/rdk/logging"
	"go.viam.com/rdk/resource"
	"go.viam.com/rdk/services/vision"
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
	Vision string

	SecondsToFeed       float64 `json:"seconds_to_feed"`
	MinutesBetweenFeeds int     `json:"minutes_between_feeds"`
	StartHour           int     `json:"start_hour"`
	EndHour             int     `json:"end_hour"`
}

func (cfg *feederConfig) fix() {
	if cfg.MinutesBetweenFeeds <= 0 {
		cfg.MinutesBetweenFeeds = 60
	}

	if cfg.SecondsToFeed <= 0 {
		cfg.SecondsToFeed = 3
	}

	if cfg.StartHour <= 0 {
		cfg.StartHour = 7
	}

	if cfg.EndHour <= 0 {
		cfg.EndHour = 25
	}

}

func (cfg feederConfig) Validate(path string) ([]string, error) {
	deps := []string{cfg.Motor, cfg.Camera, cfg.Vision}

	if cfg.Motor == "" {
		return nil, utils.NewConfigValidationFieldRequiredError(path, "motor")
	}

	if cfg.Camera == "" {
		return nil, utils.NewConfigValidationFieldRequiredError(path, "camera")
	}

	if cfg.Vision == "" {
		return nil, utils.NewConfigValidationFieldRequiredError(path, "vision")
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

	theMotor         motor.Motor
	theVisionService vision.Service

	debug map[string]interface{}

	lastFed time.Time
}

func newFeeder(ctx context.Context, deps resource.Dependencies, config resource.Config, logger logging.Logger) (resource.Resource, error) {
	newConf, err := resource.NativeConfig[*feederConfig](config)
	if err != nil {
		return nil, err
	}
	newConf.fix()

	f := &feeder{config: newConf, name: config.ResourceName(), logger: logger, debug: map[string]interface{}{}}

	m, err := deps.Lookup(motor.Named(f.config.Motor))
	if err != nil {
		return nil, err
	}
	f.theMotor = m.(motor.Motor)

	s, err := deps.Lookup(vision.Named(f.config.Vision))
	if err != nil {
		return nil, err
	}
	f.theVisionService = s.(vision.Service)

	go f.run()

	return f, nil
}

func (f *feeder) Name() resource.Name {
	return f.name
}

func (f *feeder) run() {
	f.backgroundContext, f.backgroundCancel = context.WithCancel(context.Background())

	for {
		err := f.doLoop(f.backgroundContext)
		if err != nil {
			f.logger.Errorf("error doing feeder loop: %v", err)
		}

		if !utils.SelectContextOrWait(f.backgroundContext, 10*time.Minute) {
			f.logger.Errorf("stopping feeder")
			return
		}
	}
}

func (f *feeder) doLoop(ctx context.Context) error {
	f.logger.Infof("feeder doLoop called")
	if time.Since(f.lastFed) < time.Duration((time.Minute * time.Duration(f.config.MinutesBetweenFeeds))) {
		f.logger.Infof("not feeding because fed @ %v", f.lastFed)
		return nil
	}

	now := time.Now()
	if now.Hour() < f.config.StartHour || now.Hour() >= f.config.EndHour {
		f.logger.Infof("not feeding because not in window %v < %v <= v", f.config.StartHour, now.Hour(), f.config.EndHour)
		return nil
	}

	f.logger.Infof("checking bowl")
	_, err := f.check(ctx)
	return err
}

func (f *feeder) feed(ctx context.Context) error {
	err := f.theMotor.SetPower(ctx, .5, nil)
	if err != nil {
		return err
	}

	f.lastFed = time.Now()

	time.Sleep(time.Duration(float64(time.Second) * f.config.SecondsToFeed))
	return f.theMotor.Stop(ctx, nil)
}

// return if it fed them or not
func (f *feeder) check(ctx context.Context) (bool, error) {
	f.debug = map[string]interface{}{
		"last_check": fmt.Sprintf("%v", time.Now()),
	}
	res, err := f.theVisionService.ClassificationsFromCamera(ctx, f.config.Camera, 1, nil)
	if err != nil {
		f.debug["err"] = err
		return false, err
	}
	f.debug["classification"] = res
	if len(res) != 1 {
		f.debug["fed"] = false
		return false, fmt.Errorf("wrong num of classifications %v", res)
	}

	f.logger.Infof("classification result %v", res[0])

	if res[0].Label() != "empty" || res[0].Score() < .25 {
		f.debug["fed"] = false
		return false, nil
	}

	f.logger.Infof("feeding")

	f.debug["fed"] = true
	err = f.feed(ctx)
	if err != nil {
		f.debug["err"] = err
	}
	return true, err
}

func (f *feeder) DoCommand(ctx context.Context, cmd map[string]interface{}) (map[string]interface{}, error) {
	cmdName := cmd["cmd"]

	if cmdName == "feed" {
		return map[string]interface{}{}, f.feed(ctx)
	}

	if cmdName == "check" {
		fed, err := f.check(ctx)
		return map[string]interface{}{"fed": fed}, err
	}

	return f.debug, nil
}

func (f *feeder) Close(ctx context.Context) error {
	f.backgroundCancel()
	return f.theMotor.Stop(ctx, nil)
}
