package main

import (
	"fmt"

	combustion "github.com/carsonmcdonald/go-combustion"
)

func main() {
	c := &combustion.Combustion{}

	c.StartMonitoring(func(c *combustion.Combustion, packet combustion.CombustionPacket) {
		if len(packet.Temps) > 1 {
			fmt.Printf("Temps probe(%d)=%0.2f°F, surface(%d)=%0.2f°F, ambient(%d)=%0.2f°F\n",
				packet.VirtualCoreIndex, (packet.Temps[packet.VirtualCoreIndex]*9/5)+32,
				packet.VirtualSurfaceIndex, (packet.Temps[packet.VirtualSurfaceIndex]*9/5)+32,
				packet.VirtualAmbientIndex, (packet.Temps[packet.VirtualAmbientIndex]*9/5)+32)
		}
	})
}
