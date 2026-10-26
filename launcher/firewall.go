package launcher

// AllowFirewallFlag runs the program elevated (Windows asks to confirm) just
// to let BudMinder through Windows' firewall: the phone window's "Allow
// through firewall". Not for people to type.
const AllowFirewallFlag = "--allow-firewall"

// firewallCommands are the netsh command lines that let exe through the
// firewall on private networks. The first clears every rule Windows made for
// this program, including the block rule a Cancel on Windows' first "allow
// access?" box leaves behind (a block beats an allow); the second allows it.
func firewallCommands(exe string) []string {
	return []string{
		`netsh advfirewall firewall delete rule name=all program="` + exe + `"`,
		`netsh advfirewall firewall add rule name="BudMinder" dir=in action=allow program="` + exe + `" enable=yes profile=private`,
	}
}
