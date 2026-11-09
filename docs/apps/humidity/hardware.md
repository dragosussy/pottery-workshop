# humidity: hardware

Getting the sensor, pump and screen working on the Pi. General Pi setup is in
[Deployment](../../deployment.md).

## The sensor

Enable I²C before the first deploy, because the container won't start without
it:

```sh
sudo raspi-config nonint do_i2c 0
```

Then check that `/dev/i2c-1` exists.

## The pump

The pump is off (`PUMP=none`) until you set it up in `docker-compose.yml`:

```yaml
- PUMP=gpio
- PUMP_PIN=17            # BCM GPIO number (17 = physical pin 11)
- PUMP_ACTIVE_LOW=true   # most cheap relay boards switch on when their input is low
```

`PUMP_ACTIVE_LOW` has no default, because a wrong guess would run the pump
while the app shows it off. After deploying, press **Run 1 s** on the home
screen and check the pump runs for a second and stops.

Other settings (`PUMP_PULSE`, `PUMP_REST`, `PUMP_MAX_RH`, `SENSOR`,
`SENSOR_ADDRS`, `DATA_DIR`, `ADDR`) are listed in
[the design doc](design.md#configuration-environment).

## The screen

A 1.9" 170 × 320 IPS module with an ST7789 controller (4-wire SPI) can sit
next to the Pi and show the shelf's humidity and temperature. It redraws when
they change (checked every 5 s), and goes dark when the app stops.

1. Enable SPI on the Pi: `sudo raspi-config nonint do_spi 0` (or
   `raspi-config` → Interface Options → SPI).
2. Wire it to the Pi 5 header. The module's pin order varies by seller, so go
   by the labels printed next to its pins:

   | Screen pin | Pi 5 pin | Function |
   |---|---|---|
   | GND | 20 | ground |
   | VCC | 17 | 3.3 V |
   | SCL (or SCK) | 23 | GPIO11, SPI0 clock |
   | SDA (or MOSI) | 19 | GPIO10, SPI0 data |
   | RES (or RST) | 13 | GPIO27 |
   | DC | 22 | GPIO25 |
   | CS | 24 | GPIO8, SPI0 CE0 |
   | BLK (or BL) | 12 | GPIO18, backlight |

3. In `docker-compose.yml`, uncomment the `/dev/spidev0.0` device and set
   `SCREEN=st7789`. If you wired RES, DC or BLK to other pins, set
   `SCREEN_RST`, `SCREEN_DC` or `SCREEN_BL` to their BCM numbers. If the
   picture is upside down, set `SCREEN_FLIP=true`.

In `make dev` the screen is simulated: each frame is written to
`apps/humidity/data/screen.png`.
