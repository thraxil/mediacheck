FROM centurylink/ca-certs
COPY mediacheck /
ENTRYPOINT ["/mediacheck"]
