package main

import (
	"doglog/config"
	"fmt"
	"github.com/akamensky/argparse"
	"github.com/araddon/dateparse"
	"os"
	"os/user"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// DefaultLimit is the value used when no limit is provided by the user
const DefaultLimit = 300

// DefaultRange is the value used when no range is provided by the user
const DefaultRange = "2h"

// DefaultConfigPath is the default location of the configuration path.
const DefaultConfigPath = "~/.doglog"

// options structure stores the command-line options and values.
type options struct {
	service      string
	query        string
	limit        int
	tail         bool
	configPath   string
	timeRange    int
	startDate    *time.Time
	endDate      *time.Time
	json         bool
	serverConfig *config.IniFile
	color        bool
}

// parseArgs parses the command-line arguments.
// returns: *options which contains both the parsed command-line arguments.
func parseArgs() *options {
	parser := argparse.NewParser("doglog", "Search and tail logs from Datadog.")

	var defaultConfigPath = expandPath(DefaultConfigPath)

	service := parser.String("s", "service", &argparse.Options{Required: false, Help: "Special case to search the 'service' message field, e.g., -s send-email is equivalent to -q 'service:send-email'. Merged with the -q query using 'AND' if the -q query is present."})
	query := parser.String("q", "query", &argparse.Options{Required: false, Help: "Query terms to search on (Doglog search syntax). Defaults to '*'."})
	limit := parser.Int("l", "limit", &argparse.Options{Required: false, Help: "The maximum number of messages to request from Datadog. Must be greater then 0", Default: DefaultLimit})
	tail := parser.Flag("t", "tail", &argparse.Options{Required: false, Help: "Whether to tail the output. Requires a relative search."})
	configPath := parser.String("c", "config", &argparse.Options{Required: false, Help: "Path to the config file", Default: defaultConfigPath})
	timeRange := parser.String("r", "range", &argparse.Options{Required: false, Help: "Time range to search backwards from the current moment. Examples: 30m, 2h, 4d", Default: DefaultRange})
	start := parser.String("", "start", &argparse.Options{Required: false, Help: "Starting time to search from. Allows variable formats, including '1:32pm' or '1/4/2019 12:30:00'."})
	end := parser.String("", "end", &argparse.Options{Required: false, Help: "Ending time to search from. Allows variable formats, including '6:45am' or '2019-01-04 12:30:00'. Defaults to now if --start is provided but no --end."})
	json := parser.Flag("j", "json", &argparse.Options{Required: false, Help: "Output messages in json format. Shows the modified log message, not the untouched message from Datadog. Useful in understanding the fields available when creating Format templates or for further processing."})
	noColor := parser.Flag("", "no-colors", &argparse.Options{Required: false, Help: "Don't use colors in output."})

	if err := parser.Parse(os.Args); err != nil {
		invalidArgs(parser, err, "")
	}

	startDate := strToDate(parser, *start, "The --start date can't be parsed", false)
	endDate := strToDate(parser, *end, "The --end date can't be parsed", true)

	if *limit <= 0 {
		var newLimit = DefaultLimit
		limit = &newLimit
	}

	if startDate != nil {
		var newTail = false
		tail = &newTail
	}

	var newQuery string
	if len(*service) > 0 {
		newQuery = "service:" + *service
		if len(*query) > 0 {
			newQuery += " AND " + *query
		}
		query = &newQuery
	}

	opts := options{
		service:    *service,
		query:      *query,
		limit:      *limit,
		tail:       *tail,
		configPath: *configPath,
		timeRange:  timeRangeToSeconds(parser, *timeRange),
		startDate:  startDate,
		endDate:    endDate,
		json:       *json,
		color:      !*noColor && isTty(),
	}

	// Read the configuration file
	cfg, err := config.New(opts.configPath)
	if err != nil {
		invalidArgs(parser, err, "")
	}

	opts.serverConfig = cfg

	return &opts
}

// Convert a variable human-friendly date into a time.Time.
func strToDate(parser *argparse.Parser, dateStr string, errorStr string, defaultToNow bool) *time.Time {
	var dateTime time.Time
	var err error

	if len(dateStr) > 0 {
		// Check to see if the date is a time only
		matched, _ := regexp.MatchString("^[0-9]{1,2}:[0-9]{2}(:[0-9]{2})?([ ]*(am|pm|AM|PM)?)?$", dateStr)
		if matched {
			dateStr = time.Now().Format("2006-01-02") + " " + dateStr
		}
		dateTime, err = dateparse.ParseLocal(dateStr)
		if err != nil {
			invalidArgs(parser, err, errorStr)
		} else {
			if dateTime.Year() == 0 {
				dateTime = dateTime.AddDate(time.Now().Year(), 0, 0)
			}
		}
		if err != nil {
			return nil
		}
		return &dateTime
	}
	if defaultToNow {
		dateTime = time.Now()
		return &dateTime
	}
	return nil
}

// Converts a simple human-friendly time range into seconds, e.g., 2h for 2 hours, 3d2h30m for 3 days, 2 hours and
// 30 minutes.
func timeRangeToSeconds(parser *argparse.Parser, timeRange string) int {
	re := regexp.MustCompile("([0-9]*)([a-zA-Z]*)")
	parts := re.FindAllString(timeRange, -1)
	var accumulator int
	for _, part := range parts {
		if len(part) > 1 {
			unit := part[len(part)-1:]
			numberStr := part[:len(part)-1]
			num, err := strconv.Atoi(numberStr)
			if err != nil {
				invalidArgs(parser, err, "Time range can't be parsed")
			}
			switch strings.ToLower(unit) {
			case "s":
				accumulator += num
			case "m":
				accumulator += num * 60
			case "h":
				accumulator += num * 3600
			case "d":
				accumulator += num * 86400
			default:
				invalidArgs(parser, err, "Time range can't be parsed")
			}
		}
	}
	return accumulator
}

// Display the help message when a command-line argument is invalid.
func invalidArgs(parser *argparse.Parser, err error, msg string) {
	if len(msg) > 0 {
		if err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "%s: %s\n\n", msg, err.Error())
		} else {
			_, _ = fmt.Fprintf(os.Stderr, "%s\n\n", msg)
		}
	} else if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "%s\n\n", err.Error())
	}
	_, _ = fmt.Fprintf(os.Stderr, parser.Usage(nil))
	os.Exit(1)
}

// Expand a leading tilde (~) in a file path into the user's home directory.
func expandPath(configPath string) string {
	var path = configPath
	if strings.HasPrefix(configPath, "~/") {
		usr, _ := user.Current()
		dir := usr.HomeDir

		// Use strings.HasPrefix so we don't match paths like
		// "/something/~/something/"
		path = filepath.Join(dir, path[2:])
	}
	return path
}

// Check to see whether we're outputting to a terminal or if we've been redirected to a file
func isTty() bool {
	//_, err := unix.IoctlGetTermios(int(os.Stdout.Fd()), unix.TIOCGETA)
	//return err == nil
	return true
}
