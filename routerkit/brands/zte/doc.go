// Package zte is a read-only client for the web UI of ZTE ZXHN F86xx
// fiber gateways (developed against an F8648P, firmware V2.0.12P7N17).
//
// The device serves its data as XML (ParaName/ParaValue pairs) from
// /?_type=menuData&_tag=<tag>, but only after the UI page (menu view) that
// owns the tag was loaded in the same session. The Client hides this: it
// logs in on demand (SHA-256 of password+salt), loads the page, reads the
// tags, and logs in again when the session expires. See PROTOCOL.md.
//
// The client never changes the device configuration. The device allows a
// single web session per account and locks the login after repeated
// failures, so a rejected login disables automatic re-login until Login
// is called again.
package zte
