package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	cloudflare "github.com/cloudflare/cloudflare-go"
	"github.com/sirupsen/logrus"
)

type Config struct {
	APIKey   string   `json:"apiKey"`
	APIEmail string   `json:"apiEmail"`
	Interval string   `json:"interval"`
	Domains  []string `json:"domains"`
}

var config *Config

var logLevel = flag.String("log-level", "warning", "log level")
var configLocation = flag.String("config", "config.json", "location of the config file")

func main() {
	flag.Parse()
	if lvl, err := logrus.ParseLevel(*logLevel); err == nil {
		log.Println("Setting logrus level to:", lvl)
		logrus.SetLevel(lvl)
		logrus.SetFormatter(&logrus.TextFormatter{FullTimestamp: true})
	}
	fmt.Println("Current IP is:", GetOutboundIP())
	config, err := loadConfig(*configLocation)
	if err != nil {
		log.Fatal(err)
	}

	api, err := cloudflare.New(config.APIKey, config.APIEmail)
	if err != nil {
		log.Fatal(err)
	}

	dur, err := time.ParseDuration(config.Interval)
	if err != nil {
		log.Fatal(err)
		return
	}

	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGINT, syscall.SIGTERM)

	logrus.Info("Starting with sync interval: ", config.Interval)
	ticker := time.NewTicker(dur)
	sync(api, config.Domains)
	for {
		select {
		case <-ticker.C:
			sync(api, config.Domains)
		case <-c:
			ticker.Stop()
			logrus.Info("Shutting down")
			return
		}
	}
}

func sync(api *cloudflare.API, domains []string) {
	ip := GetOutboundIP()
	for _, domain := range domains {
		err := updateCloudFlare(api, domain, ip)
		if err != nil {
			logrus.Errorf("Error updating %s: %s", domain, err.Error())
		}
	}
}

func getBaseDomain(domain string) string {
	tmp := strings.Split(domain, ".")
	return tmp[len(tmp)-2] + "." + tmp[len(tmp)-1]
}

func updateCloudFlare(api *cloudflare.API, domain string, ip net.IP) error {
	// Fetch the zone ID
	id, err := api.ZoneIDByName(getBaseDomain(domain)) // Assuming example.com exists in your Cloudflare account already
	if err != nil {
		return err
	}
	foo := cloudflare.DNSRecord{Name: domain, Type: "A"}
	recs, err := api.DNSRecords(id, foo)
	if err != nil {
		return err
	}

	if len(recs) == 0 {
		logrus.Debugf("Record %s not found. Creating!", domain)
		//create the record
		record := cloudflare.DNSRecord{
			Name:    domain,
			Type:    "A",
			Content: ip.String(),
			TTL:     1,
			Proxied: false,
		}
		resp, err := api.CreateDNSRecord(id, record)
		log.Printf("response: %#v", resp)
		return err
	}

	for _, r := range recs {
		if r.Content == ip.String() {
			logrus.Debugf("Skipping update of %s since ip already is %s", r.Name, ip.String())
			continue
		}
		fmt.Printf("Updating %s with ip: %s to ip:%s\n", r.Name, r.Content, ip.String())
		r.Content = ip.String()
		api.UpdateDNSRecord(id, r.ID, r)
	}

	return nil
}

func loadConfig(fn string) (*Config, error) {
	file, err := os.Open(fn) // For read access.
	if err != nil {
		log.Fatal(err)
	}

	decoder := json.NewDecoder(file)
	config := &Config{}
	err = decoder.Decode(config)
	if config.Interval == "" {
		logrus.Warning("interval config not set. Using default 10m")
		config.Interval = "10m"
	}
	return config, err
}

// GetOutboundIP preferred outbound ip of this machine
func GetOutboundIP() net.IP {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()

	localAddr := conn.LocalAddr().(*net.UDPAddr)

	return localAddr.IP
}
