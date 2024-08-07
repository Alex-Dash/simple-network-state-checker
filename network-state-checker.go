package main

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/big"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/gorilla/mux"
)

type SysState struct {
	HealthCode *int        `json:"healthcode,omitempty"`
	Servers    *[]SrvState `json:"servers,omitempty"`
}

type SrvState struct {
	ID                   *int    `json:"server_id,omitempty"`
	DisplayName          *string `json:"display_name,omitempty"`
	ServerTests          *int    `json:"server_tests,omitempty"`
	FailedTests          *int    `json:"failed_tests,omitempty"`
	SucceededTests       *int    `json:"succeeded_tests,omitempty"`
	CurrentVerdictCode   *int    `json:"verdict_code,omitempty"`
	CurrentVerdictString *string `json:"verdict_string,omitempty"`
}

type MonServer struct {
	URL                *string `json:"url,omitempty"`
	FollowRedir        *bool   `json:"follow_redir,omitempty"`
	DisplayName        *string `json:"display_name,omitempty"`
	Type               *string `json:"type,omitempty"`
	SuccessCodes       *[]int  `json:"success_codes,omitempty"`
	CheckCode          *bool   `json:"check_code,omitempty"`
	TestCount          *int    `json:"test_count,omitempty"`
	TestDelayMs        *int    `json:"test_delay_ms,omitempty"`
	CheckPeriodSeconds *int    `json:"check_period_seconds,omitempty"`

	Critical *bool `json:"is_critical,omitempty"`
}

type MonConfig struct {
	UseCachedResults *bool        `json:"use_cached_results,omitempty"`
	CodeHealthy      int          `json:"code_healthy,omitempty"`
	CodeDegraded     int          `json:"code_degraded,omitempty"`
	CodeFailed       int          `json:"code_failed,omitempty"`
	Servers          *[]MonServer `json:"servers,omitempty"`
}

var (
	http_port string
	exPath    string
	config    MonConfig
	state     SysState
	state_bus chan SrvState
)

var (
	STATE_OK       string = "OK"
	STATE_DEGRADED string = "DEGRADED"
	STATE_FAILED   string = "FAILED"
	STATE_UNKNOWN  string = "UNKNOWN"
)

var pb []string = []string{
	"The machine with a base-plate of prefabulated aluminite, surmounted by a malleable logarithmic casing in such a way that the two main spurving bearings were in a direct line with the pentametric fan",
	"IKEA battery supplies",
	"Probably not you...",
	"php 4.0.1",
	"The smallest brainfuck interpreter written using Piet",
	"8192 monkeys with typewriters",
	"16 dumplings and one chicken nuggie",
	"Imaginary cosmic duck",
	"13 space chickens",
	" // TODO: Fill this field in",
	"Marshmallow on a stick",
	"Two sticks and a duct tape",
	"Multipolygonal eternal PNGs",
	"40 potato batteries. Embarrassing. Barely science, really.",
	"BECAUSE I'M A POTATO",
	"Aperture Science computer-aided enrichment center",
	"Fifteen Hundred Megawatt Aperture Science Heavy Duty Super-Colliding Super Button",
}

// Blackhole
func denyIncoming(w http.ResponseWriter, r *http.Request) {
	rd, e := rand.Int(rand.Reader, big.NewInt(int64(len(pb))))
	if e != nil {
		rd = big.NewInt(int64(0))
	}

	if r.Method == "GET" {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	w.Header().Set("X-Powered-By", pb[rd.Int64()])
	w.Header().Set("content-type", "text/plain")
	w.Header().Set("access-control-allow-origin", "*")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Access-Control-Allow-Headers, Authorization, X-Requested-With, Access-Key, API-usr, Token, ref-key, lu-key")
	w.WriteHeader(403)
	fmt.Fprintf(w, "403: Access denied")
}

func getEnvStr(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

func chk(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("content-type", "application/json")
	if config.UseCachedResults != nil && *config.UseCachedResults {
		if state.HealthCode != nil {
			w.WriteHeader(*state.HealthCode)
		}
		ms, err := json.Marshal(state)
		if err != nil {
			fmt.Fprint(w, "{\"error\":\"Error while trying to marshal json\"}")
			return
		}
		fmt.Fprint(w, string(ms))
		return
	} else {
		// @TODO: add on-demand checks
	}
}

func handleRequests() {
	router := mux.NewRouter().StrictSlash(false)

	router.HandleFunc("/", chk)

	router.NotFoundHandler = router.NewRoute().HandlerFunc(denyIncoming).GetHandler()

	if http_port == "443" {
		log.Fatal(http.ListenAndServeTLS(":"+http_port, exPath+"/cert.crt", exPath+"/priv.key", router))
	} else {
		log.Fatal(http.ListenAndServe(":"+http_port, router))
	}
	log.Println("Listening on :" + http_port)
}

func loadCFG() {
	http_port = getEnvStr("PORT", "80")
	ex, err := os.Executable()
	if err != nil {
		panic(err)
	}
	exPath = filepath.Dir(ex)

	content, err := os.ReadFile(exPath + "/check.json")
	if err != nil {
		log.Fatal("Fileopen err: ", err)
	}

	err = json.Unmarshal(content, &config)
	if err != nil {
		log.Fatal("Unmarshal fail: ", err)
	}
}

func init_chk() {

	if config.Servers == nil {
		log.Fatal("servers array expected in check.json")
	}
	dummy_srvs := make([]SrvState, len(*config.Servers))
	state.Servers = &dummy_srvs
	for idx, srv := range *config.Servers {
		if srv.CheckPeriodSeconds == nil || (srv.TestDelayMs == nil && srv.TestCount != nil && *srv.TestCount > 1) {
			log.Printf("Could not satisfy loop conditions for server ID: %d. Skipping...\n", idx)
			continue
		}
		if srv.Type == nil {
			log.Printf("Type for server ID %d was not set. Skipping...\n", idx)
			continue
		}
		go func(msrv MonServer, midx int) {
			for {
				br := false
				switch true {
				case *msrv.Type == "http" || *msrv.Type == "https":
					if msrv.URL == nil {
						log.Printf("Server ID %d is missing URL, cannot proceed.\n", midx)
						br = true
						break
					}
					no_redir_cli := &http.Client{
						CheckRedirect: func(req *http.Request, via []*http.Request) error {
							return http.ErrUseLastResponse
						},
					}
					req, err := http.NewRequest(http.MethodGet, *msrv.URL, nil)
					if err != nil {
						log.Printf("http request: could not create request: %s\n", err)
						break
					}
					if msrv.TestCount == nil {
						t1 := 1
						msrv.TestCount = &t1
					}

					break_all := false
					t0, t01, t02, t03 := 0, 0, 0, 0
					unk := STATE_UNKNOWN
					test_state := SrvState{
						ID:                   &midx,
						DisplayName:          msrv.DisplayName,
						ServerTests:          &t0,
						FailedTests:          &t01,
						SucceededTests:       &t02,
						CurrentVerdictCode:   &t03,
						CurrentVerdictString: &unk,
					}

					for i := 0; i < *msrv.TestCount; i++ {
						var res *http.Response
						if msrv.FollowRedir != nil && !*msrv.FollowRedir {
							res, err = no_redir_cli.Do(req)
						} else {
							res, err = http.DefaultClient.Do(req)
						}
						*test_state.ServerTests++

						if err != nil {
							log.Printf("http request: error making http request: %s\n", err)
							*test_state.FailedTests++
							break
						}

						_, err := io.ReadAll(res.Body)
						if err != nil {
							log.Printf("http request: could not read response body: %s\n", err)
							*test_state.FailedTests++
							break
						}

						// access test results
						if msrv.CheckCode != nil && *msrv.CheckCode {
							if msrv.SuccessCodes == nil {
								log.Printf("Check status code flag was set for server ID %d, but no valid status codes were defined\n", midx)
								*test_state.FailedTests++
								break_all = true
								break
							} else {
								found := false
								for _, succ_sc := range *msrv.SuccessCodes {
									if res.StatusCode == succ_sc {
										found = true
										break
									}
								}
								if found {
									*test_state.SucceededTests++
									if *test_state.CurrentVerdictString != STATE_OK && *test_state.CurrentVerdictString != STATE_UNKNOWN {
										break
									}
									*test_state.CurrentVerdictCode = res.StatusCode
									*test_state.CurrentVerdictString = STATE_OK
								} else {
									*test_state.CurrentVerdictCode = res.StatusCode
									if msrv.Critical != nil && *msrv.Critical {
										*test_state.CurrentVerdictString = STATE_FAILED
									} else {
										*test_state.CurrentVerdictString = STATE_DEGRADED
									}
									*test_state.FailedTests++
									break
								}
							}
						} else {
							*test_state.SucceededTests++
						}
						time.Sleep(time.Duration(*msrv.TestDelayMs) * time.Millisecond)
					}
					if break_all {
						br = true
					}
					state_bus <- test_state

				default:
					log.Printf("Type %s for server ID %d is not supported\n", *msrv.Type, midx)
					br = true
				}

				if br {
					break
				}
				time.Sleep(time.Duration(*msrv.CheckPeriodSeconds) * time.Second)
			}
		}(srv, idx)
	}
	go state_resolver()
}

func state_resolver() {
	log.Println("Resolver started")
	for {
		msg := <-state_bus
		idx_srvs := *state.Servers
		if idx_srvs[*msg.ID].ID == nil {
			(*state.Servers)[*msg.ID] = msg
		} else {
			*(*state.Servers)[*msg.ID].FailedTests += *msg.FailedTests
			*(*state.Servers)[*msg.ID].SucceededTests += *msg.SucceededTests
			*(*state.Servers)[*msg.ID].ServerTests += *msg.ServerTests
			if *(*state.Servers)[*msg.ID].CurrentVerdictString != *msg.CurrentVerdictString {
				log.Printf("Server state changed for ID %d (%s): %s -> %s\n", *msg.ID, *msg.DisplayName, *(*state.Servers)[*msg.ID].CurrentVerdictString, *msg.CurrentVerdictString)
			}
			*(*state.Servers)[*msg.ID].CurrentVerdictCode = *msg.CurrentVerdictCode
			*(*state.Servers)[*msg.ID].CurrentVerdictString = *msg.CurrentVerdictString
		}
		// check statuses
		worst_state := STATE_UNKNOWN
		for _, st_srv := range *state.Servers {
			if st_srv.CurrentVerdictString == nil {
				continue
			}
			switch *st_srv.CurrentVerdictString {
			case STATE_FAILED:
				worst_state = *st_srv.CurrentVerdictString
			case STATE_DEGRADED:
				if worst_state != STATE_FAILED {
					worst_state = *st_srv.CurrentVerdictString
				}
			case STATE_OK:
				if worst_state != STATE_FAILED || worst_state != STATE_DEGRADED {
					worst_state = *st_srv.CurrentVerdictString
				}
			}
		}

		// Update cluster info
		dcode := 200
		switch worst_state {
		case STATE_FAILED:
			state.HealthCode = &config.CodeFailed
		case STATE_DEGRADED:
			state.HealthCode = &config.CodeDegraded
		case STATE_OK:
			state.HealthCode = &config.CodeHealthy
		default:
			state.HealthCode = &dcode
		}
	}
}

func main() {
	// Keep the app running
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT, syscall.SIGHUP)
	loadCFG()
	state_bus = make(chan SrvState, 10)
	log.Println("Starting Network State Checker")
	log.Println(exPath)
	init_chk()
	go func() {
		handleRequests()
	}()

	<-sc
	log.Println("Stopping Network State Checker")
}
