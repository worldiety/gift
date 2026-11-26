// Package system is the operating system of a device that runs a gift
// program as its kiosk, as far as a settings app needs it: what the device
// is, its storage, its network, its updates, how it starts, its screen and
// its clock.
//
// [Open] returns the system the program runs on:
//
//   - Raspberry Pi OS (Debian 13, trixie) and Ubuntu 26.04 LTS, through the
//     services of the system on D-Bus: NetworkManager for the network,
//     systemd for the host name, the clock, the power and the units, and
//     apt-get for updates. Changes need root, which a kiosk has.
//   - macOS, read only: what the device is, its storage and its network.
//   - Anything else, iOS included, with little more than its storage.
//
// [Simulated] is a system that only pretends, with a Raspberry Pi on a
// workshop network, for tests, demos and the documentation.
//
// # Running as the kiosk
//
// [System.Install] copies the program to /usr/local/bin and makes it the
// kiosk: a systemd unit that takes the place of the display manager and
// runs the labwc compositor with the program as its only client, as root, on
// the first virtual terminal. [System.SetBoot] switches between that kiosk
// and the desktop of the distribution. Nothing else is installed, no user
// is created and no configuration of the desktop is changed, so going back
// to the desktop is turning one unit off and another on.
//
// # Network changes that undo themselves
//
// [System.Configure] changes an IPv4 configuration under a checkpoint of
// NetworkManager, which undoes the change after a timeout unless the
// returned [Change] is kept. A settings app asks the user, as a desktop
// asks after a change of the screen resolution: an address that cuts off
// the device reverts by itself.
package system
