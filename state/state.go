package state

import (
  "sync"
  "encoding/json"
  "encoding/xml"
  "github.com/bdon/jklmnt/linref"
  "github.com/bdon/jklmnt/nextbus"
  "net/http"
  "log"
  "fmt"
  "io/ioutil"
)

// The instantaneous state of a vehicle as returned by NextBus
type VehicleState struct {
  Time int `json:"time"`
  Index float64 `json:"index"`

  LatString string `json:"-"`
  LonString string `json:"-"`
}

// One inbound or outbound run of a vehicle
// TODO: with contiguous observations no more than five minutes apart
type VehicleRun struct {
  VehicleId string `json:"vehicle_id"`
  Dir nextbus.Direction `json:"dir"`
  States []VehicleState `json:"states"`
}

// The entire state of the system is a list of vehicle runs.
// It also has bookkeeping so it knows how to add an observation to the state.
// And synchronization primitives.
// TODO: only have mutex in the http case.
type SystemState struct {
  Runs []*VehicleRun

  //Bookkeeping for vehicle ID to current run.
  CurrentRuns map[string]*VehicleRun
  Mutex sync.RWMutex
  Referencer linref.Referencer
}

func NewSystemState() *SystemState {
  retval := SystemState{}
  retval.Runs = []*VehicleRun{}
  retval.CurrentRuns = make(map[string]*VehicleRun)
  retval.Mutex = sync.RWMutex{}
  retval.Referencer = linref.NewReferencer("102909")
  return &retval
}

func (s *SystemState) Handler(w http.ResponseWriter, r *http.Request) {
    s.Mutex.RLock()
    result, err := json.Marshal(s.Runs)
    if err != nil {
      log.Println(err)
    }
    s.Mutex.RUnlock()
    w.Header().Set("Content-Type", "application/json")
    fmt.Fprintf(w, string(result))
}

// Must be called in chronological order
func (s *SystemState) AddResponse(foo nextbus.Response, unixtime int) {
  for _, report := range foo.Reports {
    if report.LeadingVehicleId != "" {
      continue
    }

    index := s.Referencer.Reference(report.Lat(), report.Lon())
    newState := VehicleState{Index:index, Time:unixtime - report.SecsSinceReport,LatString:report.LatString, LonString:report.LonString}

    c := s.CurrentRuns[report.VehicleId]
    if c != nil {
      lastState := c.States[len(c.States)-1]
      if lastState.LatString != newState.LatString || lastState.LonString != newState.LonString {
        c.States = append(c.States, newState)
      }
    } else {
      newRun := VehicleRun{VehicleId: report.VehicleId}
      newRun.States = append(newRun.States, newState)
      s.Runs = append(s.Runs,&newRun)
      s.CurrentRuns[newRun.VehicleId] = &newRun
    }
  }
}

func (s *SystemState) Tick(unixtime int) {
  log.Println("Fetching from NextBus...")
  response := nextbus.Response{}
  get, _ := http.Get("http://webservices.nextbus.com/service/publicXMLFeed?command=vehicleLocations&a=sf-muni&r=N&t=0")
  defer get.Body.Close()
  str, _ := ioutil.ReadAll(get.Body)
  xml.Unmarshal(str, &response)

  s.Mutex.Lock()
  s.AddResponse(response, unixtime)
  log.Println(len(s.Runs))
  s.Mutex.Unlock()
  log.Println("Done Fetching.")
}
