package main

import (
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"

	"github.com/kokora3/zion/internal/genesisfreeze"
)

func runGenesisCommand(args []string, output io.Writer) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: zionctl genesis keys|build|verify")
	}
	switch args[0] {
	case "keys":
		flags := flag.NewFlagSet("genesis keys", flag.ContinueOnError)
		flags.SetOutput(io.Discard)
		outDirectory := flags.String("out-dir", "", "operator-only validator directory")
		manifest := flags.String("manifest", "", "public validator manifest output")
		genesisTime := flags.String("genesis-time", "", "frozen UTC timestamp")
		if err := flags.Parse(args[1:]); err != nil || flags.NArg() != 0 || *outDirectory == "" || *manifest == "" || *genesisTime == "" {
			return fmt.Errorf("usage: zionctl genesis keys --out-dir DIR --manifest FILE --genesis-time YYYY-MM-DDTHH:MM:SSZ")
		}
		created, err := genesisfreeze.GenerateOperatorKeys(*outDirectory, *manifest, *genesisTime)
		if err != nil {
			return err
		}
		fmt.Fprintf(output, "generated %d validator identities; private material remains under %s\n", len(created.Validators), *outDirectory)
		fmt.Fprintf(output, "wrote public-only manifest to %s\n", *manifest)
		return nil
	case "build":
		flags := flag.NewFlagSet("genesis build", flag.ContinueOnError)
		flags.SetOutput(io.Discard)
		manifest := flags.String("manifest", "", "public validator manifest")
		out := flags.String("out", "", "CometBFT genesis output")
		idOut := flags.String("id-out", "", "GenesisID output")
		if err := flags.Parse(args[1:]); err != nil || flags.NArg() != 0 || *manifest == "" || *out == "" || *idOut == "" {
			return fmt.Errorf("usage: zionctl genesis build --manifest FILE --out FILE --id-out FILE")
		}
		result, err := genesisfreeze.BuildFiles(*manifest, *out, *idOut)
		if err != nil {
			return err
		}
		fmt.Fprintf(output, "NetworkID: %s\nGenesisID: %s\nValidators: %d\n",
			result.Genesis.NetworkID, hex.EncodeToString(result.Genesis.GenesisID.Digest), len(result.Genesis.Validators))
		return nil
	case "verify":
		flags := flag.NewFlagSet("genesis verify", flag.ContinueOnError)
		flags.SetOutput(io.Discard)
		manifest := flags.String("manifest", "", "public validator manifest")
		genesis := flags.String("genesis", "", "CometBFT genesis")
		idFile := flags.String("genesis-id", "", "GenesisID file")
		machine := flags.Bool("json", false, "machine-readable JSON output")
		if err := flags.Parse(args[1:]); err != nil || flags.NArg() != 0 || *manifest == "" || *genesis == "" || *idFile == "" {
			return fmt.Errorf("usage: zionctl genesis verify --manifest FILE --genesis FILE --genesis-id FILE [--json]")
		}
		verified, err := genesisfreeze.VerifyFiles(*manifest, *genesis, *idFile)
		if err != nil {
			return err
		}
		if *machine {
			encoder := json.NewEncoder(output)
			encoder.SetIndent("", "  ")
			return encoder.Encode(verified)
		}
		fmt.Fprintf(output, "NetworkID: %s\nGenesisID: %s\nValidators: %d\nVoting power: equal (%d each)\nValidation: %s\n",
			verified.NetworkID, verified.GenesisID, verified.Validators, verified.VotingPower, verified.Validation)
		return nil
	default:
		return fmt.Errorf("unknown genesis command %q", args[0])
	}
}
