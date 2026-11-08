# Use routerctl with Home Assistant

routerctl publishes over MQTT with Home Assistant discovery, so entities
appear on their own.

## Set it up

1. Have an MQTT broker that Home Assistant uses (for example Mosquitto).
2. Point routerctl at it:

   ```yaml
   mqtt:
     enabled: true
     broker: tcp://mqtt:1883          # tcp://, ssl://, ws://, wss://
     username: routerctl
     password_file: /run/secrets/mqtt_password
     trackers: known                  # device_tracker for: known | all | none
   ```

3. Run `routerctl watch`. Each router appears as a device in Home Assistant.

## What you get

- **Sensors:** CPU, RAM, temperatures, uptime, WAN IP, public IP,
  download/upload rate and totals, clients online, firmware available.
- **Binary sensors:** WAN connectivity, firmware update, each mesh node.
- **Device trackers:** one per known (approved or labelled) client, with
  debounced presence. Approve devices with `routerctl devices approve <mac>`.
- **Switches and buttons:** LED, WiFi per band, block-all, speedtest, ...
  Only the actions the command policy allows get an entity, so reboot is
  hidden by default (see [configure.md](configure.md#choose-what-automations-may-do)).
- **Per-client "Block internet" switches:** turn them on with
  `mqtt.block_switches: true`.

Entities a device doesn't support are not created. Entities that are no
longer wanted are removed. When Home Assistant restarts, everything is
published again.

## Topics

`<base>` is `mqtt.base_topic` (default `routerctl`), and `<macid>` is the
MAC without colons.

| Topic | Payload |
|---|---|
| `<base>/status` | `online` / `offline` (bridge), retained |
| `<base>/<dev>/availability` | `online` / `offline` (router reachable), retained |
| `<base>/<dev>/state` | WAN, CPU, RAM, temperatures, traffic, clients, firmware (JSON), retained |
| `<base>/<dev>/client/<macid>` | client attributes (JSON), retained |
| `<base>/<dev>/client/<macid>/presence` | `home` / `not_home`, retained |
| `<base>/<dev>/node/<macid>` | mesh node (JSON), retained |
| `<base>/<dev>/event/<type>` | every event (JSON); subscribe to `.../event/+` for all |
| `<base>/<dev>/cmd/<action>` | run an action (see below) |
| `<base>/<dev>/cmd/<action>/result` | the result: `{device, action, args, ok, result, error, time}` |

## Run an action over MQTT

Publish to `<base>/<dev>/cmd/<action>`. The payload can be any of these:

```
on                                           # the action's on/off argument
mac=aa:bb:cc:dd:ee:ff state=on               # key=value pairs
{"band": "5g1", "state": "off", "_id": "r1"} # JSON; "_id" comes back in the result
PRESS                                        # an action without arguments
```

The command policy applies, so a refused action reports
`"error": {"code": "forbidden", ...}` on the result topic.
