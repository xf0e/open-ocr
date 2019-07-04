package ocrworker

import "flag"

type EngineConfig struct {
	SaveFiles bool
}

func DefaultEngineConfig() EngineConfig {

	engineConfig := EngineConfig{
		SaveFiles: false,
	}
	return engineConfig

}

type FlagFunctionEngine func()

func NoOpFlagFunctionEngine() FlagFunctionEngine {
	return func() {}
}

func DefaultConfigFlagsEngineOverride(flagFunction FlagFunctionEngine) EngineConfig {
	engineConfig := DefaultEngineConfig()

	flagFunction()
	var (
		SaveFiles bool
	)
	flag.BoolVar(
		&SaveFiles,
		"save_files",
		false,
		"if set there will be no clean up of temporary files",
	)

	flag.Parse()

	return engineConfig
}
