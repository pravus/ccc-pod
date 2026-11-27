package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"gopkg.in/yaml.v3"
)

type Spec struct {
	Args       []string `json:"args" yaml:"args"`
	Dns        []string `json:"dns" yaml:"dns"`
	Entrypoint *string  `json:"entrypoint" yaml:"entrypoint"`
	Env        []string `json:"env" yaml:"env"`
	EnvFile    *string  `json:"envfile" yaml:"envfile"`
	Image      *string  `json:"image" yaml:"image"`
	Hostname   *string  `json:"hostname" yaml:"hostname"`
	Hosts      []string `json:"hosts" yaml:"hosts"`
	Name       *string  `json:"name" yaml:"name"`
	Network    *string  `json:"network" yaml:"network"`
	Publish    []string `json:"publish" yaml:"publish"`
	ReadOnly   *bool    `json:"readonly" yaml:"readonly"`
	Replace    *bool    `json:"replace" yaml:"replace"`
	Rm         *bool    `json:"rm" yaml:"rm"`
	Volumes    []string `json:"volumes" yaml:"volumes"`
	Caps       struct {
		Add  []string `json:"add" yaml:"add"`
		Drop []string `json:"drop" yaml:"drop"`
	} `json:"caps" yaml:"caps"`
}

type Flags struct {
	Bliss       bool
	Detach      bool
	Driver      string
	File        string
	Interactive bool
	Show        bool
	Start       bool
	Stop        bool
	Verbose     bool
}

func main() {
	flags := Flags{}
	flag.BoolVar(&flags.Bliss, `bliss`, false, `who are you?`)
	flag.BoolVar(&flags.Detach, `d`, false, ``)
	flag.BoolVar(&flags.Detach, `detach`, false, `detach from the terminal`)
	flag.StringVar(&flags.Driver, `driver`, ``, `path to driver`)
	flag.StringVar(&flags.File, `f`, ``, `path to the yaml spec file`)
	flag.BoolVar(&flags.Interactive, `i`, false, `enables interactive mode`)
	flag.BoolVar(&flags.Show, `show`, false, `shows the command to be run`)
	flag.BoolVar(&flags.Start, `start`, false, `start the pod`)
	flag.BoolVar(&flags.Stop, `stop`, false, `stop the pod`)
	flag.BoolVar(&flags.Verbose, `v`, false, ``)
	flag.BoolVar(&flags.Verbose, `verbose`, false, `turn on extra logging`)
	flag.Parse()

	if flags.Start && flags.Stop {
		fatal(`ambiguous operations: only provide one of -start or -stop`)
	}
	if flags.Stop && flags.Bliss {
		fatal(`-bliss is only supported with -start`)
	}

	driver, err := driver(flags.Driver)
	if err != nil {
		fatal(`driver: %s`, err)
	}

	var spec Spec
	if flags.File != `` {
		file, err := os.Open(flags.File)
		if err != nil {
			fatal(`%s`, err)
		}
		decoder := yaml.NewDecoder(file)
		decoder.KnownFields(true)
		if err := decoder.Decode(&spec); err != nil {
			_ = file.Close()
			fatal(`yaml: %s`, err)
		}
		if err := file.Close(); err != nil {
			fatal(`close: %s`, err)
		}
	}

	cmd := []string{driver}
	switch {
	case flags.Start:
		if flags.File == `` {
			fatal(`pod specification required for start operation`)
		}
		if spec.Image == nil || *spec.Image == `` {
			fatal(`image is required in pod specification for start operation`)
		}

		if flags.Bliss {
			if spec.Name == nil || *spec.Name == `` {
				fatal(`container name required when using -bliss for start operation`)
			}
			err := bliss(flags, driver, *spec.Name)
			if err != nil {
				fatal(`bliss: %s`, err.Error())
			}
		}

		cmd = append(cmd, `run`)
		if flags.Interactive {
			cmd = append(cmd, []string{`--interactive`, `--tty`}...)
		}
		if flags.Detach {
			cmd = append(cmd, `--detach`)
		}
		cmd = addBool(cmd, `rm`, spec.Rm)
		cmd = addBool(cmd, `replace`, spec.Replace)
		cmd = addString(cmd, `name`, spec.Name)
		cmd = addString(cmd, `network`, spec.Network)
		cmd = addString(cmd, `hostname`, spec.Hostname)
		cmd = addSlice(cmd, `add-host`, spec.Hosts)
		cmd = addSlice(cmd, `cap-add`, spec.Caps.Add)
		cmd = addSlice(cmd, `cap-drop`, spec.Caps.Drop)
		cmd = addSlice(cmd, `dns`, spec.Dns)
		cmd = addString(cmd, `env-file`, spec.EnvFile)
		cmd = addSlice(cmd, `env`, spec.Env)
		cmd = addBool(cmd, `read-only`, spec.ReadOnly)
		cmd = addSlice(cmd, `publish`, spec.Publish)
		cmd = addSlice(cmd, `volume`, spec.Volumes)
		if spec.Entrypoint != nil {
			cmd = append(cmd, `--entrypoint=`+*spec.Entrypoint)
		}
		cmd = append(cmd, *spec.Image)
		cmd = append(cmd, spec.Args...)
		cmd = append(cmd, flag.Args()...)

		if err := run(flags, driver, cmd); err != nil {
			fatal(`run: %s`, err.Error())
		}
	case flags.Stop:
		if spec.Name == nil || *spec.Name == `` {
			if len(flag.Args()) <= 0 {
				fatal(`container name required for stop operation`)
			}
			spec.Name = &flag.Args()[0]
		}

		cmd = append(cmd, `stop`)
		cmd = append(cmd, *spec.Name)

		if err := run(flags, driver, cmd); err != nil {
			fatal(`run: %s`, err.Error())
		}
	default:
		flag.Usage()
		fatal(`no operation given; provide one of -start or -stop`)
	}
}

func driver(given string) (string, error) {
	if given != `` {
		path, err := exec.LookPath(given)
		if err != nil {
			return ``, fmt.Errorf(`path lookup: %w`, err)
		}
		return path, nil
	}
	for _, bin := range []string{`podman`, `docker`} {
		if path, err := exec.LookPath(bin); err == nil {
			return path, nil
		}
	}
	return ``, fmt.Errorf(`driver not found`)
}

func bliss(flags Flags, driver string, name string) error {
	cmd := []string{driver, `rm`, name}
	if flags.Verbose || flags.Show {
		show(cmd)
	}
	if flags.Show {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	var stderr strings.Builder
	runner := exec.CommandContext(ctx, driver, cmd[1:]...)
	runner.Stdin = os.Stdin
	runner.Stdout = os.Stdout
	runner.Stderr = &stderr

	if err := runner.Run(); err != nil {
		msg := strings.ToLower(stderr.String())
		if strings.Contains(msg, `no such container`) {
			return nil
		}
		if stderr.Len() > 0 {
			fmt.Fprint(os.Stderr, stderr.String())
		}
		return fmt.Errorf(`run: %w`, err)
	}
	return nil
}

func run(flags Flags, driver string, cmd []string) error {
	if flags.Show || flags.Verbose {
		show(cmd)
	}
	if flags.Show {
		return nil
	}
	if err := syscall.Exec(driver, cmd, os.Environ()); err != nil {
		return fmt.Errorf(`exec: %s`, err)
	}
	return nil
}

func addBool(target []string, option string, condition *bool) []string {
	if condition == nil {
		return target
	}
	if *condition {
		target = append(target, `--`+option+`=true`)
	} else {
		target = append(target, `--`+option+`=false`)
	}
	return target
}

func addSlice(target []string, option string, source []string) []string {
	for _, value := range source {
		target = append(target, `--`+option+`=`+value)
	}
	return target
}

func addString(target []string, option string, value *string) []string {
	if value != nil && *value != `` {
		target = append(target, `--`+option+`=`+*value)
	}
	return target
}

func show(cmd []string) {
	quoted := make([]string, len(cmd))
	for i := range cmd {
		quoted[i] = fmt.Sprintf(`%q`, cmd[i])
	}
	fmt.Println(strings.Join(quoted, ` `))
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
