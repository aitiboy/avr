// Package gpio implements a timer/counter unit.
// Tested compatibility:
//   ATmega48/88/168 (timer 0 only)
// Untested compatability:
//   ATmega48/88/168 (timers 1 and 2)
//   ATtiny4/5/9/10
package timer

import (
    "fmt"
    "github.com/kierdavis/avr/emulator"
    "github.com/kierdavis/avr/hardware/gpio"
    "log"
)

type Timer struct {
    em                  *emulator.Emulator
    digit               uint
    controlA            uint8
    controlB            uint8
    count               uint8
    compareValA         uint8
    compareValBufferA   uint8
    compareValB         uint8
    compareValBufferB   uint8
    interruptMask       uint8
    interruptFlags      uint8
    downwards           bool // count direction
    ocPinStates         [2]bool
    ocPinCallbacks      [2]func(bool)
    logging             bool
    inhibitCompareMatch bool // set when TCNT is written to prevent a compare match on the next clock
    excessTicks         uint
}

func New(digit uint) (t *Timer) {
    return &Timer{
        digit: digit,
    }
}

func (t *Timer) SetLogging(enabled bool) {
    t.logging = enabled
}

func (t *Timer) AddTo(em *emulator.Emulator) {
    t.em = em

    em.RegisterPortByName(fmt.Sprintf("TCCR%dA", t.digit), tccra{t})
    em.RegisterPortByName(fmt.Sprintf("TCCR%dB", t.digit), tccrb{t})
    em.RegisterPortByName(fmt.Sprintf("TCNT%d", t.digit), tcnt{t})
    em.RegisterPortByName(fmt.Sprintf("OCR%dA", t.digit), ocra{t})
    em.RegisterPortByName(fmt.Sprintf("OCR%dB", t.digit), ocrb{t})
    em.RegisterPortByName(fmt.Sprintf("TIMSK%d", t.digit), timsk{t})
    em.RegisterPortByName(fmt.Sprintf("TIFR%d", t.digit), tifr{t})
}

// Connect an output-compare pin to a GPIO port by calling the GPIO's
// OverrideOutput method.
func (t *Timer) OverrideOCPin(ocPinNum uint, gpioPinNum uint, g *gpio.GPIO) {
    t.ocPinCallbacks[ocPinNum] = g.OverrideOutput(gpioPinNum)
}

// Note: in PWM modes, OCRA/OCRB do not exhibit a newly written value until the count overflows

func (t *Timer) Run(ticks uint) {
    var ticksIncr uint

    switch t.controlB & 0x07 {
    case 0: // disabled
        t.excessTicks = 0
        return
    case 1: // divider = 1
        ticksIncr = 1
    case 2: // divider = 8
        ticksIncr = 8
    case 3: // divider = 64
        ticksIncr = 64
    case 4: // divider = 256
        ticksIncr = 256
    case 5: // divider = 1024
        ticksIncr = 1024
    case 6, 7:
        panic("(*Timer).Run: external clock sources not implemented")
    }
    ticksExecuted := t.excessTicks

    for ticksExecuted < ticks {
        t.Tick()
        ticksExecuted += ticksIncr
    }

    t.excessTicks = ticksExecuted - ticks
}

// Tick the timer.
func (t *Timer) Tick() {
    // Handle match-compare interrupts
    if t.inhibitCompareMatch {
        t.inhibitCompareMatch = false
    } else {
        if t.count == t.compareValA {
            t.setOCF(0)
        }
        if t.count == t.compareValB {
            t.setOCF(1)
        }
    }

    // Prepare to tick counter
    switch t.getWGM() {
    case 0: // Normal
        t.tickNormalMode()
    case 1: // Phase-correct PWM (TOP = 0xFF)
        t.tickPCPWMMode(0xFF)
    case 2: // Clear timer on compare
        t.tickCTCMode()
    case 3: // Fast PWM (TOP = 0xFF)
        t.tickFastPWMMode(0xFF)
    case 5: // Phase-correct PWM (TOP = OCRA)
        t.tickPCPWMMode(t.compareValA)
    case 7: // Fast PWM (TOP = OCRA)
        t.tickPCPWMMode(t.compareValA)
    }

    // Actually tick the counter
    if t.downwards {
        t.count--
    } else {
        t.count++
    }

    // Trigger interrupts, if possible
    if t.em != nil && t.em.InterruptsEnabled() {
        intName := ""
        if t.interruptFlags&0x01 != 0 && t.interruptMask&0x01 != 0 {
            t.interruptFlags &= 0xFE
            intName = fmt.Sprintf("TIMER%d_OVF", t.digit)
        } else if t.interruptFlags&0x02 != 0 && t.interruptMask&0x02 != 0 {
            t.interruptFlags &= 0xFD
            intName = fmt.Sprintf("TIMER%d_COMPA", t.digit)
        } else if t.interruptFlags&0x04 != 0 && t.interruptMask&0x04 != 0 {
            t.interruptFlags &= 0xFB
            intName = fmt.Sprintf("TIMER%d_COMPB", t.digit)
        }

        if intName != "" {
            ok := t.em.InterruptByName(intName)
            if !ok && t.logging {
                log.Printf("[avr/hardware/timer:(*Timer).Tick] failed to trigger interrupt %s", intName)
            }
        }
    }
}

// Tick the timer in Normal mode.
func (t *Timer) tickNormalMode() {
    // double buffering of OCRs is disabled
    t.compareValA = t.compareValBufferA
    t.compareValB = t.compareValBufferB
    
    t.downwards = false
    if t.count == 0xFF { // Overflow
        t.setTOV()
    }

    t.checkOCPinNormalMode(0, t.compareValA)
    t.checkOCPinNormalMode(1, t.compareValB)
}

// Check for output-compare in normal mode.
func (t *Timer) checkOCPinNormalMode(ocPinNum uint, compareVal uint8) {
    if t.count == compareVal {
        t.changeOCPinNormalMode(ocPinNum)
    }
}

func (t *Timer) changeOCPinNormalMode(ocPinNum uint) {
    // Get COMxy bits
    switch t.getCOM(ocPinNum) {
    case 0: // OCy disabled
        // do nothing
    case 1: // toggle OCy
        t.toggleOCPin(ocPinNum)
    case 2: // clear OCy
        t.clearOCPin(ocPinNum)
    case 3: // set OCy
        t.setOCPin(ocPinNum)
    }
}

// Tick the timer in phase-corrected PWM mode.
func (t *Timer) tickPCPWMMode(top uint8) {
    // double buffering of OCRs is enabled
    if t.count == top {
        t.compareValA = t.compareValBufferA
        t.compareValB = t.compareValBufferB
    }
    
    if t.downwards {
        if t.count == 0x00 { // Reached BOTTOM
            t.downwards = false // Begin counting upwards
            t.setTOV()
        }
    } else {
        if t.count == top { // Reached TOP
            t.downwards = true // Begin counting downwards
        }
    }

    t.checkOCPinPCPWMMode(0, t.compareValA)
    t.checkOCPinPCPWMMode(1, t.compareValB)
}

// Check for output-compare in phase-corrected PWM mode.
func (t *Timer) checkOCPinPCPWMMode(ocPinNum uint, compareVal uint8) {
    if t.count == compareVal {
        // Get COMxy bits
        switch t.getCOM(ocPinNum) {
        case 0: // OCy disabled
            // do nothing
        case 1: // Toggle OCy (only on OC pin 0 with WGM2 bit set)
            if ocPinNum == 0 && (t.controlB&0x08) != 0 {
                t.toggleOCPin(ocPinNum)
            }
        case 2: // Clear OCy if counting upwards or set OCy if counting downwards
            if t.downwards {
                t.setOCPin(ocPinNum)
            } else {
                t.clearOCPin(ocPinNum)
            }
        case 3: // Set OCy if counting upwards or clear OCy if counting downwards
            if t.downwards {
                t.clearOCPin(ocPinNum)
            } else {
                t.setOCPin(ocPinNum)
            }
        }
    }
}

// Tick the timer in clear-timer-on-compare mode.
func (t *Timer) tickCTCMode() {
    // double buffering of OCRs is disabled
    t.compareValA = t.compareValBufferA
    t.compareValB = t.compareValBufferB
    
    t.downwards = false

    if t.count == 0xFF { // Overflow
        t.setTOV()
    }

    if t.count == t.compareValA {
        // this tick should set counter to 0
        t.count = 0xFF
    }

    t.checkOCPinCTCMode(0, t.compareValA)
    t.checkOCPinCTCMode(1, t.compareValB)
}

// Check for output-compare in normal mode.
func (t *Timer) checkOCPinCTCMode(ocPinNum uint, compareVal uint8) {
    if t.count == compareVal {
        t.changeOCPinCTCMode(ocPinNum)
    }
}

func (t *Timer) changeOCPinCTCMode(ocPinNum uint) {
    // Get COMxy bits
    switch t.getCOM(ocPinNum) {
    case 0: // OCy disabled
        // do nothing
    case 1: // toggle OCy
        t.toggleOCPin(ocPinNum)
    case 2: // clear OCy
        t.clearOCPin(ocPinNum)
    case 3: // set OCy
        t.setOCPin(ocPinNum)
    }
}

// Tick the timer in fast PWM mode.
func (t *Timer) tickFastPWMMode(top uint8) {
    // double buffering of OCRs is enabled
    if t.count == 0x00 {
        t.compareValA = t.compareValBufferA
        t.compareValB = t.compareValBufferB
    }
    
    t.downwards = false

    if t.count == top {
        // this tick should set counter to 0
        t.count = 0xFF
        t.setTOV()
    }

    t.checkOCPinFastPWMMode(0, t.compareValA)
    t.checkOCPinFastPWMMode(1, t.compareValB)
}

// Check for output-compare in fast PWM mode.
func (t *Timer) checkOCPinFastPWMMode(ocPinNum uint, compareVal uint8) {
    // BOTTOM
    if t.count == 0x00 {
        // Get COMxy bits
        switch t.getCOM(ocPinNum) {
        case 0: // OCy disabled
            // do nothing
        case 1: // Toggle OCy on compare match (only on OC pin 0 with WGM2 bit set)
            // do nothing
        case 2: // Clear OCy on compare match, set OCy at BOTTOM
            t.setOCPin(ocPinNum)
        case 3: // Set OCy on compare match, clear OCy at BOTTOM
            t.clearOCPin(ocPinNum)
        }
    }

    // Compare match
    if t.count == compareVal {
        // Get COMxy bits
        switch t.getCOM(ocPinNum) {
        case 0: // OCy disabled
            // do nothing
        case 1: // Toggle OCy on compare match (only on OC pin 0 with WGM2 bit set)
            if ocPinNum == 0 && (t.controlB&0x08) != 0 {
                t.toggleOCPin(ocPinNum)
            }
        case 2: // Clear OCy on compare match, set OCy at BOTTOM
            t.clearOCPin(ocPinNum)
        case 3: // Set OCy on compare match, clear OCy at BOTTOM
            t.setOCPin(ocPinNum)
        }
    }
}

// Force an output compare by calling the appropriate changeOCPin method.
func (t *Timer) forceOutputCompare(ocPinNum uint) {
    switch t.getWGM() {
    case 0: // Normal
        t.changeOCPinNormalMode(ocPinNum)
    case 2: // Clear timer on compare
        t.changeOCPinCTCMode(ocPinNum)
    default:
        // Force output compare has no effect in non-PWM modes.
    }
}

// Toggle an output-compare pin.
func (t *Timer) toggleOCPin(ocPinNum uint) {
    t.ocPinStates[ocPinNum] = !t.ocPinStates[ocPinNum]
    t.updateOCPin(ocPinNum)
}

// Set an output-compare pin to low.
func (t *Timer) clearOCPin(ocPinNum uint) {
    if t.ocPinStates[ocPinNum] {
        t.ocPinStates[ocPinNum] = false
        t.updateOCPin(ocPinNum)
    }
}

// Set an output-compare pin to high.
func (t *Timer) setOCPin(ocPinNum uint) {
    if !t.ocPinStates[ocPinNum] {
        t.ocPinStates[ocPinNum] = true
        t.updateOCPin(ocPinNum)
    }
}

// Push the new status of an output-compare pin to the GPIO layer.
func (t *Timer) updateOCPin(ocPinNum uint) {
    callback := t.ocPinCallbacks[ocPinNum]
    if callback != nil {
        callback(t.ocPinStates[ocPinNum])
    }
}

// Set an output compare match (OCFy) flag.
func (t *Timer) setOCF(ocPinNum uint) {
    if ocPinNum == 0 { // A
        t.interruptFlags |= 0x02
    } else { // B
        t.interruptFlags |= 0x04
    }
}

// Set the timer overflow (TOV) flag.
func (t *Timer) setTOV() {
    t.interruptFlags |= 0x01
}

// Get the WGM (waveform generation mode) bits
func (t *Timer) getWGM() (wgm uint8) {
    wgmA := t.controlA & 0x03        // bits 1 and 0
    wgmB := (t.controlB & 0x08) >> 1 // bit 3 (shifted to bit 2)
    return wgmB | wgmA
}

// Get the COM (compare output mode) bits for a given OC pin number.
func (t *Timer) getCOM(ocPinNum uint) (com uint8) {
    shiftAmt := 6 - 2*ocPinNum // 0 => 6, 1 => 4
    return (t.controlA >> shiftAmt) & 0x03
}
