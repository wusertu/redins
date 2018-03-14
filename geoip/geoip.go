package geoip

import (
    "math"
    "net"
    "log"

    "github.com/oschwald/maxminddb-golang"
    "github.com/go-ini/ini"
    "github.com/hawell/redins/handler"
)

type GeoIp struct {
    config *GeoipConfig
    db     *maxminddb.Reader
}

type GeoipConfig struct {
    enable bool
    dbName string
}

func LoadConfig(cfg *ini.File, section string) *GeoipConfig {
    geoipConfig := cfg.Section(section)
    return &GeoipConfig {
        enable: geoipConfig.Key("enable").MustBool(true),
        dbName: geoipConfig.Key("db").MustString("geoCity.mmdb"),
    }
}

func NewGeoIp(config *GeoipConfig) *GeoIp {
    g := &GeoIp {
        config: config,
    }
    var err error
    if config.enable {
        g.db, err = maxminddb.Open(config.dbName)
        if err != nil {
            log.Printf("[ERROR] cannot open maxminddb file %s", err)
            g.config.enable = false
        }
    }
    // defer g.db.Close()
    return g
}

func (g *GeoIp) FilterGeoIp(sourceIp net.IP, record *handler.Record) {
    if !g.config.enable {
        return
    }
    g.GetMinimumDistance(sourceIp, record)
}

func (g *GeoIp) GetMinimumDistance(sourceIp net.IP, record *handler.Record) {
    minDistance := 1000.0
    index := -1
    slat, slong, _, err := g.GetGeoLocation(sourceIp)
    if err != nil {
        log.Printf("[ERROR] getMinimumDistance failed")
        return
    }
    for i, _ := range record.A {
        destinationIp := record.A[i].Ip
        dlat, dlong, _, err := g.GetGeoLocation(destinationIp)
        d, err := g.getDistance(slat, slong, dlat, dlong)
        if err != nil {
            continue
        }
        if d < minDistance {
            minDistance = d
            index = i
        }
    }
    if index > -1 {
        record.A = []handler.A_Record {record.A[index] }
    }
    index = -1
    for i, _ := range record.AAAA {
        destinationIp := record.AAAA[i].Ip
        dlat, dlong, _, err := g.GetGeoLocation(destinationIp)
        d, err := g.getDistance(slat, slong, dlat, dlong)
        if err != nil {
            continue
        }
        if d < minDistance {
            minDistance = d
            index = i
        }
    }
    if index > -1 {
        record.AAAA = []handler.AAAA_Record {record.AAAA[index] }
    }
}

func (g *GeoIp) getDistance(slat, slong, dlat, dlong float64) (float64, error) {
    deltaLat := (dlat - slat) * math.Pi / 180.0
    deltaLong := (dlong - slong) * math.Pi / 180.0
    slat = slat * math.Pi / 180.0
    dlat = dlat * math.Pi / 180.0

    a := math.Sin(deltaLat/2.0)*math.Sin(deltaLat/2.0) +
        math.Cos(slat)*math.Cos(dlat)* math.Sin(deltaLong/2.0)*math.Sin(deltaLong/2.0)
    c := 2.0 * math.Atan2(math.Sqrt(a), math.Sqrt(1.0-a))

    // fmt.Println("distance = ", c)

    return c, nil
}

func (g *GeoIp) GetGeoLocation(ip net.IP) (latitude float64, longitude float64, country string, err error) {
    var record struct {
        Location struct {
            Latitude        float64 `maxminddb:"latitude"`
            LongitudeOffset uintptr `maxminddb:"longitude"`
        } `maxminddb:"location"`
        Country struct {
            ISOCode string `maxminddb:"iso_code"`
        } `maxminddb:"country"`    }
    // log.Printf("ip : %s\n", ip)
    err = g.db.Lookup(ip, &record)
    if err != nil {
        log.Printf("[ERROR] lookup failed : %s", err)
        return 0, 0, "", err
    }
    g.db.Decode(record.Location.LongitudeOffset, &longitude)
    // fmt.Println("lat = ", record.Location.Latitude, " lang = ", longitude)
    return record.Location.Latitude, longitude, record.Country.ISOCode, nil
}
