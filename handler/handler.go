package handler

import (
    "encoding/json"
    "net"
    "strings"
    "time"
    "log"

    "github.com/miekg/dns"
    "github.com/hawell/redins/redis"
    "github.com/go-ini/ini"
)

type DnsRequestHandler struct {
    config         *HandlerConfig
    Zones          []string
    LastZoneUpdate time.Time
    Redis          *redis.Redis
}

type Zone struct {
    Name        string
    Locations   map[string]struct{}
}

type Record struct {
    A     []A_Record     `json:"a,omitempty"`
    AAAA  []AAAA_Record  `json:"aaaa,omitempty"`
    TXT   []TXT_Record   `json:"txt,omitempty"`
    CNAME []CNAME_Record `json:"cname,omitempty"`
    NS    []NS_Record    `json:"ns,omitempty"`
    MX    []MX_Record    `json:"mx,omitempty"`
    SRV   []SRV_Record   `json:"srv,omitempty"`
    SOA   SOA_Record     `json:"soa,omitempty"`
}

type A_Record struct {
    Ttl         uint32        `json:"ttl,omitempty"`
    Ip          net.IP        `json:"ip"`
}

type AAAA_Record struct {
    Ttl         uint32        `json:"ttl,omitempty"`
    Ip          net.IP        `json:"ip"`
}

type TXT_Record struct {
    Ttl  uint32 `json:"ttl,omitempty"`
    Text string `json:"text"`
}

type CNAME_Record struct {
    Ttl  uint32 `json:"ttl,omitempty"`
    Host string `json:"host"`
}

type NS_Record struct {
    Ttl  uint32 `json:"ttl,omitempty"`
    Host string `json:"host"`
}

type MX_Record struct {
    Ttl        uint32 `json:"ttl,omitempty"`
    Host       string `json:"host"`
    Preference uint16 `json:"preference"`
}

type SRV_Record struct {
    Ttl      uint32 `json:"ttl,omitempty"`
    Priority uint16 `json:"priority"`
    Weight   uint16 `json:"weight"`
    Port     uint16 `json:"port"`
    Target   string `json:"target"`
}

type SOA_Record struct {
    Ttl     uint32 `json:"ttl,omitempty"`
    Ns      string `json:"ns"`
    MBox    string `json:"MBox"`
    Refresh uint32 `json:"refresh"`
    Retry   uint32 `json:"retry"`
    Expire  uint32 `json:"expire"`
    MinTtl  uint32 `json:"minttl"`
}

type HandlerConfig struct {
    redisConfig *redis.RedisConfig
    ttl         uint32
}

func LoadConfig(cfg *ini.File, section string) *HandlerConfig {
    handlerConfig := cfg.Section(section)
    return &HandlerConfig{
        redisConfig: redis.LoadConfig(cfg, section),
        ttl:         uint32(handlerConfig.Key("ttl").MustUint(360)),
    }
}

func NewHandler(config *HandlerConfig) *DnsRequestHandler {
    h := &DnsRequestHandler {
        config: config,
    }

    h.Redis = redis.NewRedis(config.redisConfig)

    h.LoadZones()

    return h
}

func (h *DnsRequestHandler) LoadZones() {
    h.LastZoneUpdate = time.Now()
    h.Zones = h.Redis.GetKeys()
}

func (h *DnsRequestHandler) A(name string, record *Record) (answers []dns.RR) {
    for _, a := range record.A {
        if a.Ip == nil {
            continue
        }
        r := new(dns.A)
        r.Hdr = dns.RR_Header{Name: name, Rrtype: dns.TypeA,
            Class: dns.ClassINET, Ttl: h.minTtl(a.Ttl)}
        r.A = a.Ip
        answers = append(answers, r)
    }
    return
}

func (h *DnsRequestHandler) AAAA(name string, record *Record) (answers []dns.RR) {
    for _, aaaa := range record.AAAA {
        if aaaa.Ip == nil {
            continue
        }
        r := new(dns.AAAA)
        r.Hdr = dns.RR_Header{Name: name, Rrtype: dns.TypeAAAA,
            Class: dns.ClassINET, Ttl: h.minTtl(aaaa.Ttl)}
        r.AAAA = aaaa.Ip
        answers = append(answers, r)
    }
    return
}

func (h *DnsRequestHandler) CNAME(name string, record *Record) (answers []dns.RR) {
    for _, cname := range record.CNAME {
        if len(cname.Host) == 0 {
            continue
        }
        r := new(dns.CNAME)
        r.Hdr = dns.RR_Header{Name: name, Rrtype: dns.TypeCNAME,
            Class: dns.ClassINET, Ttl: h.minTtl(cname.Ttl)}
        r.Target = dns.Fqdn(cname.Host)
        answers = append(answers, r)
    }
    return
}

func (h *DnsRequestHandler) TXT(name string, record *Record) (answers []dns.RR) {
    for _, txt := range record.TXT {
        if len(txt.Text) == 0 {
            continue
        }
        r := new(dns.TXT)
        r.Hdr = dns.RR_Header{Name: name, Rrtype: dns.TypeTXT,
            Class: dns.ClassINET, Ttl: h.minTtl(txt.Ttl)}
        r.Txt = split255(txt.Text)
        answers = append(answers, r)
    }
    return
}

func (h *DnsRequestHandler) NS(name string, record *Record) (answers []dns.RR) {
    for _, ns := range record.NS {
        if len(ns.Host) == 0 {
            continue
        }
        r := new(dns.NS)
        r.Hdr = dns.RR_Header{Name: name, Rrtype: dns.TypeNS,
            Class: dns.ClassINET, Ttl: h.minTtl(ns.Ttl)}
        r.Ns = ns.Host
        answers = append(answers, r)
    }
    return
}

func (h *DnsRequestHandler) MX(name string, record *Record) (answers []dns.RR) {
    for _, mx := range record.MX {
        if len(mx.Host) == 0 {
            continue
        }
        r := new(dns.MX)
        r.Hdr = dns.RR_Header{Name: name, Rrtype: dns.TypeMX,
            Class: dns.ClassINET, Ttl: h.minTtl(mx.Ttl)}
        r.Mx = mx.Host
        r.Preference = mx.Preference
        answers = append(answers, r)
    }
    return
}

func (h *DnsRequestHandler) SRV(name string, record *Record) (answers []dns.RR) {
    for _, srv := range record.SRV {
        if len(srv.Target) == 0 {
            continue
        }
        r := new(dns.SRV)
        r.Hdr = dns.RR_Header{Name: name, Rrtype: dns.TypeSRV,
            Class: dns.ClassINET, Ttl: h.minTtl(srv.Ttl)}
        r.Target = srv.Target
        r.Weight = srv.Weight
        r.Port = srv.Port
        r.Priority = srv.Priority
        answers = append(answers, r)
    }
    return
}

func (h *DnsRequestHandler) SOA(name string, zone string, record *Record) (answers []dns.RR) {
    r := new(dns.SOA)
    if record.SOA.Ns == "" {
        r.Hdr = dns.RR_Header{Name: name, Rrtype: dns.TypeSOA,
            Class: dns.ClassINET, Ttl: h.config.ttl}
        r.Ns = "ns1." + name
        r.Mbox = hostmaster + "." + name
        r.Refresh = 86400
        r.Retry = 7200
        r.Expire = 3600
        r.Minttl = uint32(h.config.ttl)
    } else {
        r.Hdr = dns.RR_Header{Name: zone, Rrtype: dns.TypeSOA,
            Class: dns.ClassINET, Ttl: h.minTtl(record.SOA.Ttl)}
        r.Ns = record.SOA.Ns
        r.Mbox = record.SOA.MBox
        r.Refresh = record.SOA.Refresh
        r.Retry = record.SOA.Retry
        r.Expire = record.SOA.Expire
        r.Minttl = record.SOA.MinTtl
    }
    r.Serial = h.serial()
    answers = append(answers, r)
    return
}

func (h *DnsRequestHandler) serial() uint32 {
    return uint32(time.Now().Unix())
}

func (h *DnsRequestHandler) minTtl(ttl uint32) uint32 {
    if h.config.ttl == 0 && ttl == 0 {
        return defaultTtl
    }
    if h.config.ttl == 0 {
        return ttl
    }
    if ttl == 0 {
        return h.config.ttl
    }
    if h.config.ttl < ttl {
        return h.config.ttl
    }
    return ttl
}

func (h *DnsRequestHandler) findLocation(query string, z *Zone) string {
    var (
        ok                                 bool
        closestEncloser, sourceOfSynthesis string
    )

    // request for zone records
    if query == z.Name {
        return query
    }

    query = strings.TrimSuffix(query, "."+z.Name)

    if _, ok = z.Locations[query]; ok {
        return query
    }

    closestEncloser, sourceOfSynthesis, ok = splitQuery(query)
    for ok {
        ceExists := keyMatches(closestEncloser, z) || keyExists(closestEncloser, z)
        ssExists := keyExists(sourceOfSynthesis, z)
        if ceExists {
            if ssExists {
                return sourceOfSynthesis
            } else {
                return ""
            }
        } else {
            closestEncloser, sourceOfSynthesis, ok = splitQuery(closestEncloser)
        }
    }
    return ""
}

func keyExists(key string, z *Zone) bool {
    _, ok := z.Locations[key]
    return ok
}

func keyMatches(key string, z *Zone) bool {
    for value := range z.Locations {
        if strings.HasSuffix(value, key) {
            return true
        }
    }
    return false
}

func splitQuery(query string) (string, string, bool) {
    if query == "" {
        return "", "", false
    }
    var (
        splits            []string
        closestEncloser   string
        sourceOfSynthesis string
    )
    splits = strings.SplitAfterN(query, ".", 2)
    if len(splits) == 2 {
        closestEncloser = splits[1]
        sourceOfSynthesis = "*." + closestEncloser
    } else {
        closestEncloser = ""
        sourceOfSynthesis = "*"
    }
    return closestEncloser, sourceOfSynthesis, true
}

func split255(s string) []string {
    if len(s) < 255 {
        return []string{s}
    }
    sx := []string{}
    p, i := 0, 255
    for {
        if i <= len(s) {
            sx = append(sx, s[p:i])
        } else {
            sx = append(sx, s[p:])
            break

        }
        p, i = p+255, i+255
    }

    return sx
}

func (h *DnsRequestHandler) Matches(qname string) string {
    zone := ""
    for _, zname := range h.Zones {
        if dns.IsSubDomain(zname, qname) {
            // We want the *longest* matching zone, otherwise we may end up in a parent
            if len(zname) > len(zone) {
                zone = zname
            }
        }
    }
    return zone
}

func (h *DnsRequestHandler) GetRecord(qname string) (record *Record, rcode int, zone string) {
    log.Printf("[INFO] GetRecord")

    log.Println(h.Zones)
    if time.Since(h.LastZoneUpdate) > zoneUpdateTime {
        log.Printf("[INFO] loading zones")
        h.LoadZones()
    }
    log.Println(h.Zones)

    zone = h.Matches(qname)
    log.Printf("[INFO] zone : %s", zone)
    if zone == "" {
        log.Printf("[ERROR] no matching zone found for %s", zone)
        return nil, dns.RcodeNameError, ""
    }

    z := h.LoadZone(zone)
    if z == nil {
        log.Printf("[ERROR] empty zone : %s", zone)
        return nil, dns.RcodeServerFailure, ""
    }

    location := h.findLocation(qname, z)
    if len(location) == 0 { // empty, no results
        return nil, dns.RcodeNameError, ""
    }
    log.Printf("[INFO] location : %s", location)

    record = h.GetLocation(location, z)

    return record, dns.RcodeSuccess, zone
}

func (h *DnsRequestHandler) LoadZone(zone string) *Zone {
    z := new(Zone)
    z.Name = zone
    vals := h.Redis.GetHKeys(zone)
    z.Locations = make(map[string]struct{})
    for _, val := range vals {
        z.Locations[val] = struct{}{}
    }

    return z
}

func (h *DnsRequestHandler) GetLocation(location string, z *Zone) *Record {
    var label string
    if location == z.Name {
        label = "@"
    } else {
        label = location
    }
    val := h.Redis.HGet(z.Name, label)
    r := new(Record)
    err := json.Unmarshal([]byte(val), r)
    if err != nil {
        log.Printf("[ERROR] cannot parse json : %s -> %s", val, err)
        return nil
    }
    return r
}

func (h *DnsRequestHandler) SetLocation(location string, z *Zone, val *Record) {
    jsonValue, err := json.Marshal(val)
    if err != nil {
        log.Printf("[ERROR] cannot encode to json : %s", err)
        return
    }
    var label string
    if location == z.Name {
        label = "@"
    } else {
        label = location
    }
    h.Redis.HSet(z.Name, label, string(jsonValue))
}

const (
    defaultTtl     = 360
    hostmaster     = "hostmaster"
    zoneUpdateTime = 10 * time.Minute
)
