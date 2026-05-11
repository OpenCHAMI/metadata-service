# SPDX-FileCopyrightText: 2025 OpenCHAMI Contributors
#
# SPDX-License-Identifier: MIT

FROM gcr.io/distroless/static-debian12:nonroot

WORKDIR /app

ARG TARGETPLATFORM

# With GoReleaser dockers_v2, binaries are available under $TARGETPLATFORM/.
COPY $TARGETPLATFORM/metadata-service /usr/local/bin/metadata-service
COPY $TARGETPLATFORM/metadata-client /usr/local/bin/metadata-client

USER nonroot:nonroot
EXPOSE 8080 9090

ENTRYPOINT ["/usr/local/bin/metadata-service"]
CMD ["serve"]
