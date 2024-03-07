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

docker:
	./scripts/build.sh $@

