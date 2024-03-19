ifeq ($(GO),)
GO := go
endif

build:
	GO=$(GO) ./scripts/build.sh

clean:
	rm -rf result

tidy:
	cd src && $(GO) mod tidy

test:
	GO=$(GO) ./scripts/build.sh test

docker-standalone:
	./scripts/build.sh $@

docker-delegation-verify:
	./scripts/build.sh $@

