# SPDX-FileCopyrightText: 2025 OpenCHAMI Contributors
#
# SPDX-License-Identifier: MIT

FROM gcr.io/distroless/static-debian12:nonroot

WORKDIR /app

ARG TARGETPLATFORM

# With GoReleaser dockers_v2, binaries are available under $TARGETPLATFORM/.
COPY $TARGETPLATFORM/metadata-server /usr/local/bin/metadata-server

USER nonroot:nonroot
EXPOSE 8080 9090

ENTRYPOINT ["/usr/local/bin/metadata-server"]
CMD ["serve"]
