package main

import (
	"flag"
	"io/ioutil"

	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v2"
)

type config struct {
	Hangouts HangoutsConfig
}

var botanistConfig = &config{}

var (
	log                = logrus.New()
	configFileLocation *string
)

func main() {
	verbose := flag.Bool("verbose", false, "Increase logging verbosity")
	configFileLocation = flag.String("configFile", "botanist.conf", "Location of config file in YAML format")
	flag.Parse()

	configFile, err := ioutil.ReadFile(*configFileLocation)
	if err != nil {
		log.Fatalf("Error when trying to read config file at %s", *configFileLocation)
	}
	err = yaml.Unmarshal(configFile, botanistConfig)
	if err != nil {
		log.Fatalf("Error when parsing config file: %s", err)
	}
	log.Infoln("Botanist Starting.")
	log.Infof("Configuration: Credentials File: %s, Project: %s, Subscription: %s.", botanistConfig.Hangouts.CredentialsFile, botanistConfig.Hangouts.Project, botanistConfig.Hangouts.PsSubscription)

	if *verbose {
		log.SetLevel(logrus.DebugLevel)
	}

	go startPrometheusListener()

	// Actively load hangouts
	// We should make this dependent on what's in the config file in the future
	// and load only the messaging plattforms that are configured
	initHangouts()
	log.Infoln("Botanist Exiting.")
}

func persistConfigChanges() error {
	data, err := yaml.Marshal(botanistConfig)
	if err != nil {
		return err
	}
	return ioutil.WriteFile(*configFileLocation, data, 0600)
}
