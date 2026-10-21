package main

import (
	"bytes"
	"io"
	"os"
	"regexp"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/mab-go/logging"

	"github.com/spf13/viper"
)

// cleanStop is the log of a server that started and stopped cleanly at the default level.
var cleanStop = []string{"server.ready", "server.stopping", "server.stopped"}

// redirectStdio points os.Stdin at a pipe and os.Stderr at a temp file until the test ends, as a
// shell redirect would for the binary, so cobra's errors and the server's log land in one place.
// It returns the pipe's write end, which the caller closes to send EOF, and a func that returns
// what has been written to stderr so far. Cleanup puts both back, then resets the logging
// package's default config, which RunE replaces; logging's own default is Info to stderr.
func redirectStdio(t *testing.T) (stdin *os.File, stderr func() string) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	errFile, err := os.CreateTemp(t.TempDir(), "stderr-*")
	if err != nil {
		t.Fatal(err)
	}
	savedIn, savedErr := os.Stdin, os.Stderr
	os.Stdin, os.Stderr = r, errFile
	t.Cleanup(func() {
		os.Stdin, os.Stderr = savedIn, savedErr
		logging.SetDefaultConfig(logging.Config{Level: logging.InfoLevel})
		_ = r.Close()
		_ = w.Close()
		_ = errFile.Close()
	})
	return w, func() string {
		b, err := os.ReadFile(errFile.Name())
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
}

// executeCmd runs the root command with args, as the binary would, and returns what cobra wrote
// to its stdout and what the run wrote to stderr. Stdin is empty, so a run that starts the server
// sees EOF at once and returns. Cleanup undoes what a run leaves in package-level state.
func executeCmd(t *testing.T, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	stdin, readStderr := redirectStdio(t)
	if err := stdin.Close(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(resetCmd)

	var outBuf bytes.Buffer
	cmd.SetOut(&outBuf)
	// A nil slice makes cobra parse os.Args[1:], the test binary's -test.* flags.
	cmd.SetArgs(append([]string{}, args...))
	err = cmd.Execute()
	return outBuf.String(), readStderr(), err
}

// resetCmd undoes what a run of cmd leaves behind. cobra keeps --version and --help set between
// runs, and viper prefers a changed --log-level to the env var and otherwise falls back to the
// flag's current value, not its default, so each flag gets back both its default value and
// Changed false. viper.Reset is safe: cobra.OnInitialize reapplies the env and flag settings on
// the next run that reaches RunE.
func resetCmd() {
	for _, name := range []string{"log-level", "version", "help"} {
		f := cmd.Flags().Lookup(name)
		if f == nil {
			f = cmd.PersistentFlags().Lookup(name)
		}
		if f == nil {
			continue // version and help exist only after the first Execute
		}
		_ = f.Value.Set(f.DefValue)
		f.Changed = false
	}
	viper.Reset()
	cmd.SetArgs(nil)
	cmd.SetOut(nil)
}

// eventPattern matches the event field of a log line.
var eventPattern = regexp.MustCompile(`event=(\S+)`)

// loggedEvents returns the names of the events in log, in order.
func loggedEvents(log string) []string {
	var events []string
	for _, m := range eventPattern.FindAllStringSubmatch(log, -1) {
		events = append(events, m[1])
	}
	return events
}

// waitForLog waits until stderr() contains want, failing the test after five seconds.
func waitForLog(t *testing.T, stderr func() string, want string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !strings.Contains(stderr(), want) {
		if time.Now().After(deadline) {
			t.Fatalf("stderr = %q, want %q within 5s", stderr(), want)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// TestParseLogLevel checks parseLogLevel accepts the four level names in any case and with
// surrounding whitespace, and rejects anything else with an error that quotes the input.
func TestParseLogLevel(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want logging.Level
	}{
		{"debug", logging.DebugLevel},
		{"info", logging.InfoLevel},
		{"warn", logging.WarnLevel},
		{"error", logging.ErrorLevel},
		{"DEBUG", logging.DebugLevel},
		{"Warn", logging.WarnLevel},
		{" error ", logging.ErrorLevel},
		{"\tinfo\n", logging.InfoLevel},
	} {
		if got, err := parseLogLevel(tc.in); err != nil || got != tc.want {
			t.Errorf("parseLogLevel(%q) = %v, %v; want %v, nil", tc.in, got, err, tc.want)
		}
	}
	// fatal and panic are logging levels, but not ones the server can be set to.
	for _, in := range []string{"", "bogus", "warning", "fatal", "panic", "trace"} {
		if _, err := parseLogLevel(in); err == nil || !strings.Contains(err.Error(), strconv.Quote(in)) {
			t.Errorf("parseLogLevel(%q) error = %v, want an error quoting %s", in, err, strconv.Quote(in))
		}
	}
}

// TestVersionFlag checks --version and -v print the version line and return without starting
// the server. go test passes no -ldflags, so the version variables keep their defaults.
func TestVersionFlag(t *testing.T) {
	t.Setenv("XMIND_MCP_LOG_LEVEL", "")
	for _, arg := range []string{"--version", "-v"} {
		t.Run(arg, func(t *testing.T) {
			stdout, stderr, err := executeCmd(t, arg)
			if err != nil {
				t.Fatalf("Execute(%s) = %v, want nil", arg, err)
			}
			if want := "xmind-mcp 0.0.0 (000000000000; 0000-00-00)\n"; stdout != want {
				t.Errorf("stdout = %q, want %q", stdout, want)
			}
			if stderr != "" {
				t.Errorf("stderr = %q, want nothing", stderr)
			}
		})
	}
}

// TestInvalidLogLevelFlag checks an unknown --log-level fails the command with an error naming
// the value. Each test uses its own invalid value, so one leaked from another test can't pass.
func TestInvalidLogLevelFlag(t *testing.T) {
	t.Setenv("XMIND_MCP_LOG_LEVEL", "")
	_, stderr, err := executeCmd(t, "--log-level", "verbose")
	if err == nil || !strings.Contains(err.Error(), `"verbose"`) {
		t.Fatalf(`Execute = %v, want an invalid log level error naming "verbose"`, err)
	}
	if !strings.Contains(stderr, "Error: invalid log level") {
		t.Errorf("stderr = %q, want it to report the invalid log level", stderr)
	}
}

// TestInvalidLogLevelEnv checks an unknown XMIND_MCP_LOG_LEVEL fails the command, as an unknown
// --log-level does.
func TestInvalidLogLevelEnv(t *testing.T) {
	t.Setenv("XMIND_MCP_LOG_LEVEL", "loud")
	if _, _, err := executeCmd(t); err == nil || !strings.Contains(err.Error(), `"loud"`) {
		t.Fatalf(`Execute = %v, want an invalid log level error naming "loud"`, err)
	}
}

// TestRunStopsAtEOF checks a valid level starts the server, which returns nil when stdin closes,
// and that the level it logs at comes from the flag, else the env var. At info the log shows the
// server ran and stopped cleanly; at error it shows nothing, which the default info would not.
func TestRunStopsAtEOF(t *testing.T) {
	for _, tc := range []struct {
		name string
		env  string
		args []string
		want []string // the events logged
	}{
		{"flag", "", []string{"--log-level", "info"}, cleanStop},
		{"env", "error", nil, nil},
		{"flag over env", "error", []string{"--log-level", "info"}, cleanStop},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("XMIND_MCP_LOG_LEVEL", tc.env)
			stdout, stderr, err := executeCmd(t, tc.args...)
			if err != nil {
				t.Fatalf("Execute = %v, want nil once stdin closes", err)
			}
			// Empty output also rules out a --version left set by an earlier run.
			if stdout != "" {
				t.Errorf("stdout = %q, want nothing", stdout)
			}
			if got := loggedEvents(stderr); !slices.Equal(got, tc.want) {
				t.Errorf("logged events = %v, want %v; stderr = %q", got, tc.want, stderr)
			}
		})
	}
}

// TestRunStopsOnSignal checks an interrupt or SIGTERM stops a running server cleanly. Stdin stays
// open, so only the signal can stop it. Without RunStdioServer's handler, the signal would end
// the test binary.
func TestRunStopsOnSignal(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("os.Process.Signal can't send an interrupt or SIGTERM on Windows")
	}
	t.Setenv("XMIND_MCP_LOG_LEVEL", "")
	self, err := os.FindProcess(os.Getpid())
	if err != nil {
		t.Fatal(err)
	}
	for _, sig := range []os.Signal{os.Interrupt, syscall.SIGTERM} {
		t.Run(sig.String(), func(t *testing.T) {
			stdin, stderr := redirectStdio(t)
			t.Cleanup(resetCmd)
			cmd.SetOut(io.Discard)
			cmd.SetArgs([]string{"--log-level", "info"})

			var runErr error
			finished := make(chan struct{})
			go func() {
				defer close(finished)
				runErr = cmd.Execute()
			}()
			t.Cleanup(func() {
				_ = stdin.Close() // EOF stops a server the signal didn't, before stdio is restored
				<-finished
			})

			// The handler is registered before server.ready is logged.
			waitForLog(t, stderr, "event=server.ready")
			if err := self.Signal(sig); err != nil {
				t.Fatal(err)
			}
			select {
			case <-finished:
			case <-time.After(5 * time.Second):
				t.Fatalf("Execute did not return within 5s of %v", sig)
			}
			if runErr != nil {
				t.Fatalf("Execute = %v, want nil after %v", runErr, sig)
			}
			if got := loggedEvents(stderr()); !slices.Equal(got, cleanStop) {
				t.Errorf("logged events = %v, want %v", got, cleanStop)
			}
		})
	}
}
