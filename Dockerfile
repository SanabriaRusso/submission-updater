FROM golang:1.20

# Set the Current Working Directory inside the container
WORKDIR $GOPATH/src

# Copy everything from the current directory to the PWD (Present Working Directory) inside the container
COPY src src

# Download all the dependencies
RUN cd src && go get -d -v ./...

# Install the package
RUN cd src && go install -v ./...

# Run the executable
ENTRYPOINT ["cassandra_updater"]
