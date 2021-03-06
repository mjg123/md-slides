package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gorilla/mux"

	"github.com/AstromechZA/md-slides/sliderenderer"
)

const serveUsage = `Usage:
  md-slides serve [options...] <filepath>

Options:
`

func parseResString(i string) (int, int, error) {
	i = strings.TrimSpace(strings.ToLower(i))
	parts := strings.Split(i, "x")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("res string '%s' did not contain one 'x'", i)
	}
	xres, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, fmt.Errorf("failed to parse x value of res string '%s': %s", i, err)
	}
	yres, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0, 0, fmt.Errorf("failed to parse y value of res string '%s': %s", i, err)
	}
	if xres <= 0 {
		return 0, 0, fmt.Errorf("x value of rest string '%s' should be > 0", i)
	}
	if yres <= 0 {
		return 0, 0, fmt.Errorf("y value of rest string '%s' should be > 0", i)
	}
	return int(xres), int(yres), nil
}

func Serve(args []string) error {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, serveUsage)
		fs.PrintDefaults()
	}
	hotFlag := fs.Bool("hot", false, "reload, reparse, and regenerate slides on each refresh")
	checkOnlyFlag := fs.Bool("check-only", false, "stop after checking slides")
	resFlag := fs.String("res", "1366x768", "set render aspect ratio and zoom for rendering")
	fontSizeFlag := fs.Int("font-size", 18, "relative font size within slide")
	portFlag := fs.Int("port", 8080, "port to listen on")
	hostFlag := fs.String("host", "", "host to listen on (localhost, 127.0.0.1)")
	backgroundCSS := fs.String("css-background", "#fffff8", "slide background css")
	noStaticsFlag := fs.Bool("no-statics", false, "disable static file serving (security option)")
	exportFlag := fs.String("export-to", "", "export slides as a single html page (dont serve)")
	modeFlag := fs.String("mode", "paged", "mode to serve in (paged|scrolling)")

	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		fs.Usage()
		fmt.Fprintf(os.Stderr, "\n")
		return fmt.Errorf("expected a single source file as argument")
	}
	filename := fs.Arg(0)

	xres, yres, err := parseResString(*resFlag)
	if err != nil {
		return fmt.Errorf("bad res string: %s", err)
	}

	sr := sliderenderer.SlideRenderer{
		Filename: filename,
		Hot:      *hotFlag,
		XRes:     xres,
		YRes:     yres,
		FontSize: *fontSizeFlag,
		BGCSS:    *backgroundCSS,
		URLPath:  "/_slides",
	}
	if err = sr.Init(); err != nil {
		return fmt.Errorf("failed to init renderer: %s", err)
	}
	if err = sr.CheckSlides(); err != nil {
		return fmt.Errorf("check failed: %s", err)
	}
	if *checkOnlyFlag {
		return nil
	}
	if *exportFlag != "" {
		log.Printf("Attempting to export html to %s", *exportFlag)
		rec := httptest.NewRecorder()
		sr.MultiServeHTTP(rec, httptest.NewRequest(http.MethodGet, "http://local/", nil))
		if rec.Code != http.StatusOK {
			return fmt.Errorf("Unexpected failure")
		}
		if *exportFlag == "-" {
			if _, err := io.Copy(os.Stdout, rec.Body); err != nil {
				return fmt.Errorf("IO error: %s", err)
			}
		} else {
			f, err := os.Create(*exportFlag)
			if err != nil {
				return fmt.Errorf("failed to export: %s", err)
			}
			if _, err := io.Copy(f, rec.Body); err != nil {
				return fmt.Errorf("IO error: %s", err)
			}
			if err := f.Close(); err != nil {
				return fmt.Errorf("IO error: %s", err)
			}
		}
		return nil
	}

	r := mux.NewRouter()
	switch *modeFlag {
	case "paged":
		r.Path(sr.URLPath + "/").Handler(http.RedirectHandler(sr.FirstSlidePath(), http.StatusTemporaryRedirect))
		r.Path(sr.URLPath).HandlerFunc(sr.ServeHTTP)
	case "scrolling":
		r.Path(sr.URLPath).HandlerFunc(sr.MultiServeHTTP)
	default:
		return fmt.Errorf("unknown mode '%s'", *modeFlag)
	}
	if !*noStaticsFlag {
		r.Path("/{static}").Handler(http.FileServer(CustomDirFS{Directory: filepath.Dir(filename)}))
	}
	r.Path("/").Handler(http.RedirectHandler(sr.URLPath, http.StatusTemporaryRedirect))

	listenString := net.JoinHostPort(*hostFlag, strconv.Itoa(*portFlag))
	log.Printf("Ready to serve on %s", listenString)
	if err := http.ListenAndServe(listenString, r); err != nil {
		return err
	}
	return nil
}
