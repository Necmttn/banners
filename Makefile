build:
	-mkdir -p ./bin > /dev/null 2>&1;
	go build
	-rm ./bin/banners > /dev/null 2>&1
	mv ./banners ./bin

example:
	sudo masscan --rate 100000 -p$(PORT) --exclude 255.255.255.255 0.0.0.0/0 | awk '{ split($$4, pm, "/"); print "{\"Port\":"pm[1]",\"Ip\":\""$$6"\"}"; fflush();   }' | ./bin/banners -geoip ./data/GeoLite2-City.mmdb -config ./defaults/config.json -data ./defaults/banner -concurrent 5

