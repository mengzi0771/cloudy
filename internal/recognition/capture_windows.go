//go:build windows && (amd64 || arm64)

package recognition

import (
	"context"
	"encoding/binary"
	"fmt"
	"math"
	"runtime"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

const Available = true

var (
	ole32            = windows.NewLazySystemDLL("ole32.dll")
	coInitialize     = ole32.NewProc("CoInitializeEx")
	coUninitialize   = ole32.NewProc("CoUninitialize")
	coCreateInstance = ole32.NewProc("CoCreateInstance")
	coTaskMemFree    = ole32.NewProc("CoTaskMemFree")
	deviceEnumerator = audioGUID("BCDE0395-E52F-467C-8E3D-C4579291692E")
	iidEnumerator    = audioGUID("A95664D2-9614-4F35-A746-DE8DB63617E6")
	iidAudioClient   = audioGUID("1CB9AD4C-DBFA-4c32-B178-C2F568A703B2")
	iidCaptureClient = audioGUID("C8ADBD64-E71E-48a0-A4DE-185C395CD317")
)

func audioGUID(s string) windows.GUID {
	id, err := windows.GUIDFromString("{" + s + "}")
	if err != nil {
		panic(err)
	}
	return id
}

// Vtable indices include the three IUnknown methods at the start.
const (
	release            = 2
	getDefaultEndpoint = 4 // IMMDeviceEnumerator
	activate           = 3 // IMMDevice
	initialize         = 3 // IAudioClient
	getMixFormat       = 8
	start              = 10
	stop               = 11
	getService         = 14
	getBuffer          = 3 // IAudioCaptureClient
	releaseBuffer      = 4
	getNextPacketSize  = 5
)

type audioObject struct{ vtable *[15]uintptr }

func audioError(method string, hr uintptr) error {
	if int32(hr) < 0 {
		return fmt.Errorf("%s: HRESULT 0x%08x", method, uint32(hr))
	}
	return nil
}

// Capture reads the mix going to the default Windows playback device via
// WASAPI loopback. All COM calls for the stream stay on one thread.
func Capture(ctx context.Context) ([]float32, int, error) {
	ctx, cancel := context.WithTimeout(ctx, (Seconds+2)*time.Second)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return nil, 0, err
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	hr, _, _ := coInitialize.Call(0, 2) // COINIT_APARTMENTTHREADED
	if err := audioError("CoInitializeEx", hr); err != nil {
		return nil, 0, err
	}
	defer coUninitialize.Call()

	var enumerator, device, client, capture *audioObject
	hr, _, _ = coCreateInstance.Call(uintptr(unsafe.Pointer(&deviceEnumerator)), 0, 1,
		uintptr(unsafe.Pointer(&iidEnumerator)), uintptr(unsafe.Pointer(&enumerator)))
	if err := audioError("CoCreateInstance", hr); err != nil {
		return nil, 0, err
	}
	defer syscall.SyscallN(enumerator.vtable[release], uintptr(unsafe.Pointer(enumerator)))
	hr, _, _ = syscall.SyscallN(enumerator.vtable[getDefaultEndpoint], uintptr(unsafe.Pointer(enumerator)),
		0, 1, uintptr(unsafe.Pointer(&device))) // eRender, eMultimedia
	if err := audioError("GetDefaultAudioEndpoint", hr); err != nil {
		return nil, 0, err
	}
	defer syscall.SyscallN(device.vtable[release], uintptr(unsafe.Pointer(device)))
	hr, _, _ = syscall.SyscallN(device.vtable[activate], uintptr(unsafe.Pointer(device)),
		uintptr(unsafe.Pointer(&iidAudioClient)), 1, 0, uintptr(unsafe.Pointer(&client)))
	if err := audioError("IMMDevice.Activate", hr); err != nil {
		return nil, 0, err
	}
	defer syscall.SyscallN(client.vtable[release], uintptr(unsafe.Pointer(client)))
	var format *byte
	hr, _, _ = syscall.SyscallN(client.vtable[getMixFormat], uintptr(unsafe.Pointer(client)), uintptr(unsafe.Pointer(&format)))
	if err := audioError("GetMixFormat", hr); err != nil {
		return nil, 0, err
	}
	defer coTaskMemFree.Call(uintptr(unsafe.Pointer(format)))
	mix, err := readMixFormat(format)
	if err != nil {
		return nil, 0, err
	}
	hr, _, _ = syscall.SyscallN(client.vtable[initialize], uintptr(unsafe.Pointer(client)), 0,
		0x00020000, 2_000_000, 0, uintptr(unsafe.Pointer(format)), 0) // shared, loopback, 200 ms buffer
	if err := audioError("IAudioClient.Initialize", hr); err != nil {
		return nil, 0, err
	}
	hr, _, _ = syscall.SyscallN(client.vtable[getService], uintptr(unsafe.Pointer(client)),
		uintptr(unsafe.Pointer(&iidCaptureClient)), uintptr(unsafe.Pointer(&capture)))
	if err := audioError("GetService", hr); err != nil {
		return nil, 0, err
	}
	defer syscall.SyscallN(capture.vtable[release], uintptr(unsafe.Pointer(capture)))
	hr, _, _ = syscall.SyscallN(client.vtable[start], uintptr(unsafe.Pointer(client)))
	if err := audioError("IAudioClient.Start", hr); err != nil {
		return nil, 0, err
	}
	defer syscall.SyscallN(client.vtable[stop], uintptr(unsafe.Pointer(client)))

	pcm := make([]float32, 0, mix.rate*Seconds)
	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()
	for len(pcm) < cap(pcm) {
		select {
		case <-ctx.Done():
			if ctx.Err() == context.DeadlineExceeded {
				return nil, 0, ErrNoAudio
			}
			return nil, 0, ctx.Err()
		case <-tick.C:
			pcm, err = readAudioPackets(capture, mix, pcm)
			if err != nil {
				return nil, 0, err
			}
		}
	}
	for _, sample := range pcm {
		if sample > 0.0005 || sample < -0.0005 {
			return pcm, mix.rate, nil
		}
	}
	return nil, 0, ErrNoAudio
}

type mixFormat struct {
	rate, channels, frameBytes, sampleBytes int
	floatingPoint                           bool
}

func readMixFormat(format *byte) (mixFormat, error) {
	b := unsafe.Slice(format, 18) // packed WAVEFORMATEX
	tag := binary.LittleEndian.Uint16(b)
	if tag == 0xfffe && binary.LittleEndian.Uint16(b[16:]) >= 22 {
		// WAVEFORMATEXTENSIBLE stores the base format in its SubFormat GUID.
		tag = binary.LittleEndian.Uint16(unsafe.Slice(format, 40)[24:])
	}
	m := mixFormat{
		channels: int(binary.LittleEndian.Uint16(b[2:])), rate: int(binary.LittleEndian.Uint32(b[4:])),
		frameBytes: int(binary.LittleEndian.Uint16(b[12:])), sampleBytes: int(binary.LittleEndian.Uint16(b[14:])) / 8,
		floatingPoint: tag == 3,
	}
	if m.channels == 0 || m.rate < 8000 || m.rate > 384000 || m.frameBytes != m.channels*m.sampleBytes ||
		!(tag == 3 && m.sampleBytes == 4 || tag == 1 && m.sampleBytes >= 2 && m.sampleBytes <= 4) {
		return mixFormat{}, fmt.Errorf("unsupported WASAPI mix format: tag %d, %+v", tag, m)
	}
	return m, nil
}

func readAudioPackets(capture *audioObject, mix mixFormat, pcm []float32) ([]float32, error) {
	for len(pcm) < cap(pcm) {
		var frames uint32
		hr, _, _ := syscall.SyscallN(capture.vtable[getNextPacketSize], uintptr(unsafe.Pointer(capture)), uintptr(unsafe.Pointer(&frames)))
		if err := audioError("GetNextPacketSize", hr); err != nil || frames == 0 {
			return pcm, err
		}
		var data *byte
		var flags uint32
		hr, _, _ = syscall.SyscallN(capture.vtable[getBuffer], uintptr(unsafe.Pointer(capture)),
			uintptr(unsafe.Pointer(&data)), uintptr(unsafe.Pointer(&frames)), uintptr(unsafe.Pointer(&flags)), 0, 0)
		if err := audioError("IAudioCaptureClient.GetBuffer", hr); err != nil || frames == 0 {
			return pcm, err
		}
		n := min(int(frames), cap(pcm)-len(pcm))
		from := len(pcm)
		pcm = pcm[:from+n]
		if flags&2 == 0 && data != nil { // AUDCLNT_BUFFERFLAGS_SILENT
			mix.mono(unsafe.Slice(data, n*mix.frameBytes), pcm[from:])
		}
		hr, _, _ = syscall.SyscallN(capture.vtable[releaseBuffer], uintptr(unsafe.Pointer(capture)), uintptr(frames))
		if err := audioError("IAudioCaptureClient.ReleaseBuffer", hr); err != nil {
			return nil, err
		}
	}
	return pcm, nil
}

func (m mixFormat) mono(data []byte, out []float32) {
	for i := range out {
		var sum float32
		for ch := range m.channels {
			s := data[i*m.frameBytes+ch*m.sampleBytes:]
			switch {
			case m.floatingPoint:
				sum += math.Float32frombits(binary.LittleEndian.Uint32(s))
			case m.sampleBytes == 2:
				sum += float32(int16(binary.LittleEndian.Uint16(s))) / 32768
			case m.sampleBytes == 3:
				sum += float32(int32(uint32(s[0])<<8|uint32(s[1])<<16|uint32(s[2])<<24)) / 2147483648
			default:
				sum += float32(int32(binary.LittleEndian.Uint32(s))) / 2147483648
			}
		}
		out[i] = sum / float32(m.channels)
	}
}
