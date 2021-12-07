package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"math"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"

	"github.com/takoyaki-3/go-gtfs"
	"github.com/takoyaki-3/go-gtfs/pkg"

	// "github.com/takoyaki-3/go-gtfs/stop_pattern"

	// "github.com/takoyaki-3/goraph/loader/osm"
	// "github.com/takoyaki-3/goraph/geometry/h3"
	"github.com/takoyaki-3/goraph"
	// "github.com/takoyaki-3/goraph/search"
	"github.com/takoyaki-3/goraph/geometry"
	"github.com/takoyaki-3/mapRAPTOR/loader"
	"github.com/takoyaki-3/mapRAPTOR/routing"
)

type Geometry struct {
	Type        string      `json:"type"`
	Coordinates interface{} `json:"coordinates"`
}
type Feature struct {
	Type       string            `json:"type"`
	Geometry   Geometry          `json:"geometry"`
	Properties map[string]string `json:"properties"`
}
type FeatureCollection struct {
	Type     string    `json:"type"`
	Features []Feature `json:"features"`
}

type QueryNodeStr struct {
	StopId *string  `json:"stop_id"`
	Lat    *float64 `json:"lat"`
	Lon    *float64 `json:"lon"`
	Time   *int     `json:"time"`
}
type QueryStr struct {
	Origin      QueryNodeStr `json:"origin"`
	Destination QueryNodeStr `json:"destination"`
	IsJSONOnly  bool         `json:"json_only"`
}

func GetRequestData(r *http.Request, queryStr interface{}) error {
	v := r.URL.Query()
	if v == nil {
		return errors.New("cannot get url query.")
	}
	return json.Unmarshal([]byte(v["json"][0]), queryStr)
}

type MTJNodeStr struct {
	Id    string  `json:"id"`
	Lat   float64 `json:"lat"`
	Lon   float64 `json:"lon"`
	Title string  `json:"title"`
}
type MTJLegStr struct {
	Id             string     `json:"id"`
	Uid            string     `json:"uid"`
	Oid            string     `json:"oid"`
	Title          string     `json:"title"`
	Created        string     `json:"created"`
	Issued         string     `json:"issued"`
	Available      string     `json:"available"`
	Valid          string     `json:"valid"`
	Type           string     `json:"type"`
	SubType        string     `json:"subtype"`
	FromNode       MTJNodeStr `json:"from_node"`
	ToNode         MTJNodeStr `json:"to_node"`
	Transportation string     `json:"transportation"`
	load           string     `json:"load"`
	WKT            string     `json:WKT`
	Geometry       Geometry   `json:"geometry"`
}

func main() {

	raptorData, g := loader.LoadGTFS()

	StopId2Index := map[string]int{}
	for i, stop := range g.Stops {
		StopId2Index[stop.ID] = i
	}

	mapStops := map[string]int{}
	for i,s := range g.Stops{
		mapStops[s.ID] = i
	}

	// index.html
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		bytes, _ := ioutil.ReadFile("./index.html")
		fmt.Fprintln(w, string(bytes))
	})
	http.HandleFunc("/routing", func(w http.ResponseWriter, r *http.Request) {

		var query QueryStr
		err := GetRequestData(r, &query)
		if err != nil {
			log.Fatalln(err)
		}

		// Query
		Round := 10

		q := &routing.Query{
			ToStop:      FindODNode(query.Destination, g),
			FromStop:    FindODNode(query.Origin, g),
			FromTime:    *query.Origin.Time,
			MinuteSpeed: 80,
			Round:       Round,
			LimitTime:   *query.Origin.Time + 36000,
		}
		memo := routing.RAPTOR(raptorData, q)

		pos := q.ToStop
		ro := Round - 1

		legs := []MTJLegStr{}

		fmt.Println("---")
		lastTime := memo.Tau[ro][pos].ArrivalTime
		for pos != q.FromStop {
			bef := memo.Tau[ro][pos]
			now := pos
			if bef.ArrivalTime == 0 {
				fmt.Println("not found !")
				break
			}
			fmt.Println(bef, pkg.Sec2HHMMSS(bef.ArrivalTime), pkg.Sec2HHMMSS(lastTime), g.Stops[StopId2Index[bef.BeforeStop]].Name, "->", g.Stops[StopId2Index[pos]].Name)
			lastTime = bef.ArrivalTime
			pos = bef.BeforeStop

			uuidObj, _ := uuid.NewUUID()
			id := uuidObj.String()
			legs = append(legs, MTJLegStr{
				FromNode: MTJNodeStr{
					Id:    string(bef.BeforeStop),
					Lat:   g.Stops[StopId2Index[pos]].Latitude,
					Lon:   g.Stops[StopId2Index[pos]].Longitude,
					Title: g.Stops[StopId2Index[bef.BeforeStop]].Name,
				},
				ToNode: MTJNodeStr{
					Id:    string(now),
					Lat:   g.Stops[StopId2Index[now]].Latitude,
					Lon:   g.Stops[StopId2Index[now]].Longitude,
					Title: g.Stops[StopId2Index[now]].Name,
				},
				Transportation: string(memo.Tau[ro][now].BeforeEdge),
				Id:             id,
				Uid:            id,
				Oid:            id,
				Created:        time.Now().Format("2006-01-02 15:04:05"),
				Geometry: Geometry{
					Type: "LineString",
					Coordinates: [][]float64{
						[]float64{g.Stops[StopId2Index[bef.BeforeStop]].Longitude, g.Stops[StopId2Index[bef.BeforeStop]].Latitude},
						[]float64{g.Stops[StopId2Index[now]].Longitude, g.Stops[StopId2Index[now]].Latitude},
					},
				},
			})
			ro = ro - 1
		}

		type TripStr struct {
			Legs []MTJLegStr `json:"legs"`
		}
		type Resp struct {
			Trips []TripStr `json:"trips"`
		}
		rawJson, _ := json.Marshal(Resp{
			Trips: []TripStr{
				TripStr{
					Legs: legs,
				},
			},
		})
		w.Write(rawJson)
	})
	http.HandleFunc("/routing_geojson", func(w http.ResponseWriter, r *http.Request) {

		var query QueryStr
		err := GetRequestData(r, &query)
		if err != nil {
			log.Fatalln(err)
		}

		// Query
		Round := 10

		q := &routing.Query{
			ToStop:      FindODNode(query.Destination, g),
			FromStop:    FindODNode(query.Origin, g),
			FromTime:    *query.Origin.Time,
			MinuteSpeed: 80,
			Round:       Round,
			LimitTime:   *query.Origin.Time + 36000,
		}
		memo := routing.RAPTOR(raptorData, q)

		ro := Round - 1

		fc := FeatureCollection{
			Type: "FeatureCollection",
		}
		for stopId,m := range memo.Tau[ro]{
			s := g.Stops[mapStops[stopId]]
			props := map[string]string{}
			props["time"] = strconv.Itoa(m.ArrivalTime-q.FromTime)
			props["arrival_time"] = pkg.Sec2HHMMSS(m.ArrivalTime)
			props["stop_id"] = stopId
			props["name"] = s.Name
			tr := ro
			for tr >= 0{
				if memo.Tau[tr][stopId].ArrivalTime != m.ArrivalTime{
					break
				}
				tr--
			}
			props["transfer"] = strconv.Itoa(tr)
			fc.Features = append(fc.Features, Feature{
				Type: "Feature",
				Geometry: Geometry{
					Type: "Point",
					Coordinates: []float64{s.Longitude,s.Latitude},
				},
				Properties: props,
			})
		}
		
		rawJson, _ := json.Marshal(fc)
		w.Write(rawJson)
	})
	fmt.Println("start server.")
	http.ListenAndServe("0.0.0.0:8000", nil)
}

func FindODNode(qns QueryNodeStr, g *gtfs.GTFS) string {
	stopId := ""
	minD := math.MaxFloat64
	if qns.StopId == nil {
		for _, stop := range g.Stops {
			d := geometry.HubenyDistance(goraph.LatLon{
				Lat: stop.Latitude,
				Lon: stop.Longitude},
				goraph.LatLon{
					Lat: *qns.Lat,
					Lon: *qns.Lon})
			if d < minD {
				stopId = stop.ID
				minD = d
			}
		}
	} else {
		stopId = *qns.StopId
	}
	return stopId
}
