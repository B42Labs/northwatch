// Package ovsdb manages Northwatch's libovsdb connections to the OVN Northbound
// and Southbound databases and to per-chassis Open_vSwitch instances.
//
// It owns connection establishment and failover (Connect), staged monitor setup
// (MonitorOptions / MonitorAll), the per-chassis OVS connection pool (OVSPool)
// and TLS configuration (BuildTLSConfig). The generated OVSDB model structs live
// in the nb, sb and vs subpackages.
//
// It is the lowest layer of the codebase and must not import higher layers such
// as internal/api. Shared, dependency-free helpers that operate on the generated
// models — ModelToMap, ModelsToMaps and DerefStr — live here so every consumer
// (search, correlate, events, write, impact, …) can use them without importing
// the HTTP layer.
package ovsdb
