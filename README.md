# canopy_server_comm
[![Build Status](https://travis-ci.org/canopy-ros/canopy_server_comm.svg?branch=master)](https://travis-ci.org/canopy-ros/canopy_server_comm)

Client communication server for Canopy

### Configuration Options
Configuration options for the communication server can be specified in a `config.*` file in any of the following formats: JSON, TOML, YAML, HCL, or Java properties. The config file may be placed in the top level of this directory or in `/etc/canopy/`.

The following keys and values are supported in the config file:
- `db`: `redis`, `none`
