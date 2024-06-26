# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

N/A

## [v1.0.0] - 2024-03-27

### Added
- Add `WithMeterProvider` as a new Option.
- Implement basic http server metrics. 
- Add example of initialization of `MeterProvider` in the example.
- Add README.md.

## [v1.0.1] - 2024-04-06

### Fixed
- Fix the bug when middleware calculating the size of the request body. (#1)

## [v1.0.2] - 2024-04-13

### Fixed
- Use semconvutil.HTTPServerRequestMetrics() for metric attrs to avoid the memory bloat. (#3)
