using System;
using System.Runtime.InteropServices;

namespace WebratCs.Microphone
{
    public class WaveInMic : IDisposable
    {
        public static string[] ListDevices()
        {
            try
            {
                uint n = waveInGetNumDevs();
                var list = new System.Collections.Generic.List<string>();
                for (uint i = 0; i < n; i++)
                {
                    WAVEINCAPS caps;
                    int ok = waveInGetDevCaps(i, out caps, (uint)Marshal.SizeOf(typeof(WAVEINCAPS)));
                    if (ok == 0)
                    {
                        var nm = caps.szPname;
                        if (!string.IsNullOrEmpty(nm)) list.Add(nm.Trim());
                    }
                }
                return list.ToArray();
            }
            catch
            {
                return new string[0];
            }
        }
        private IntPtr hWave = IntPtr.Zero;
        private IntPtr hEvent = IntPtr.Zero;
        private int sampleRate;
        private int channels;
        private int chunkMs;
        private int nBuf = 8;
        private WAVEHDR[] hdrs;
        private byte[][] bufs;
        private GCHandle[] pins;
        public event Action<byte[]> OnChunk;

        public bool Start(int sampleRate, int channels, int chunkMs, string preferredName = null)
        {
            this.sampleRate = sampleRate <= 0 ? 48000 : sampleRate;
            this.channels = (channels == 1 || channels == 2) ? channels : 1;
            this.chunkMs = Math.Max(5, chunkMs <= 0 ? 20 : chunkMs);
            var wf = new WAVEFORMATEX
            {
                wFormatTag = 1,
                nChannels = (ushort)this.channels,
                nSamplesPerSec = (uint)this.sampleRate,
                wBitsPerSample = 16,
                nBlockAlign = (ushort)(this.channels * 2),
                nAvgBytesPerSec = (uint)(this.sampleRate * this.channels * 2),
                cbSize = 0
            };
            hEvent = CreateEventW(IntPtr.Zero, false, false, null);
            IntPtr deviceID = new IntPtr(-1);
            if (!string.IsNullOrEmpty(preferredName))
            {
                try
                {
                    uint n = waveInGetNumDevs();
                    for (uint i = 0; i < n; i++)
                    {
                        WAVEINCAPS caps;
                        int ok = waveInGetDevCaps(i, out caps, (uint)Marshal.SizeOf(typeof(WAVEINCAPS)));
                        if (ok == 0)
                        {
                            var nm = (caps.szPname ?? "").Trim();
                            if (string.Equals(nm, preferredName.Trim(), StringComparison.OrdinalIgnoreCase))
                            {
                                deviceID = new IntPtr((int)i);
                                break;
                            }
                        }
                    }
                }
                catch { deviceID = new IntPtr(-1); }
            }
            int r = waveInOpen(ref hWave, deviceID, ref wf, hEvent, IntPtr.Zero, 0x00050000);
            if (r != 0 || hWave == IntPtr.Zero)
            {
                if (this.sampleRate != 44100)
                {
                    wf.nSamplesPerSec = 44100;
                    wf.nAvgBytesPerSec = (uint)(wf.nSamplesPerSec * wf.nBlockAlign);
                    r = waveInOpen(ref hWave, deviceID, ref wf, hEvent, IntPtr.Zero, 0x00050000);
                }
            }
            if (r != 0 || hWave == IntPtr.Zero) return false;
            int chunkSamples = (this.sampleRate * this.chunkMs) / 1000;
            int chunkBytes = chunkSamples * this.channels * 2;
            hdrs = new WAVEHDR[nBuf];
            bufs = new byte[nBuf][];
            pins = new GCHandle[nBuf];
            for (int i = 0; i < nBuf; i++)
            {
                bufs[i] = new byte[chunkBytes];
                pins[i] = GCHandle.Alloc(bufs[i], GCHandleType.Pinned);
                hdrs[i] = new WAVEHDR
                {
                    lpData = pins[i].AddrOfPinnedObject(),
                    dwBufferLength = (uint)bufs[i].Length
                };
                waveInPrepareHeader(hWave, ref hdrs[i], (uint)Marshal.SizeOf(typeof(WAVEHDR)));
                waveInAddBuffer(hWave, ref hdrs[i], (uint)Marshal.SizeOf(typeof(WAVEHDR)));
            }
            waveInStart(hWave);
            System.Threading.ThreadPool.QueueUserWorkItem(_ => Pump());
            return true;
        }

        private void Pump()
        {
            while (hWave != IntPtr.Zero)
            {
                WaitForSingleObject(hEvent, 200);
                for (int i = 0; i < nBuf; i++)
                {
                    if ((hdrs[i].dwFlags & 0x00000001) != 0 && hdrs[i].dwBytesRecorded > 0)
                    {
                        var payload = new byte[hdrs[i].dwBytesRecorded];
                        Marshal.Copy(hdrs[i].lpData, payload, 0, (int)hdrs[i].dwBytesRecorded);
                        var handler = OnChunk;
                        if (handler != null) handler(payload);
                        hdrs[i].dwFlags = 0;
                        hdrs[i].dwBytesRecorded = 0;
                        waveInAddBuffer(hWave, ref hdrs[i], (uint)Marshal.SizeOf(typeof(WAVEHDR)));
                    }
                }
            }
        }

        public void Dispose()
        {
            if (hWave != IntPtr.Zero)
            {
                waveInStop(hWave);
                waveInReset(hWave);
                waveInClose(hWave);
                hWave = IntPtr.Zero;
            }
            if (hEvent != IntPtr.Zero)
            {
                CloseHandle(hEvent);
                hEvent = IntPtr.Zero;
            }
            if (pins != null)
            {
                for (int i = 0; i < pins.Length; i++)
                {
                    if (pins[i].IsAllocated) pins[i].Free();
                }
                pins = null;
            }
        }

        [StructLayout(LayoutKind.Sequential)]
        private struct WAVEFORMATEX
        {
            public ushort wFormatTag;
            public ushort nChannels;
            public uint nSamplesPerSec;
            public uint nAvgBytesPerSec;
            public ushort nBlockAlign;
            public ushort wBitsPerSample;
            public ushort cbSize;
        }

        [StructLayout(LayoutKind.Sequential, CharSet = CharSet.Unicode)]
        private struct WAVEINCAPS
        {
            public ushort wMid;
            public ushort wPid;
            public uint vDriverVersion;
            [MarshalAs(UnmanagedType.ByValTStr, SizeConst = 32)]
            public string szPname;
            public uint dwFormats;
            public ushort wChannels;
            public ushort wReserved1;
            public uint dwSupport;
        }

        [StructLayout(LayoutKind.Sequential)]
        private struct WAVEHDR
        {
            public IntPtr lpData;
            public uint dwBufferLength;
            public uint dwBytesRecorded;
            public IntPtr dwUser;
            public uint dwFlags;
            public uint dwLoops;
            public IntPtr lpNext;
            public IntPtr reserved;
        }

        [DllImport("winmm.dll")]
        private static extern int waveInOpen(ref IntPtr phwi, IntPtr uDeviceID, ref WAVEFORMATEX pwfx, IntPtr dwCallback, IntPtr dwInstance, uint fdwOpen);
        [DllImport("winmm.dll")]
        private static extern int waveInPrepareHeader(IntPtr hwi, ref WAVEHDR pwh, uint cbwh);
        [DllImport("winmm.dll")]
        private static extern int waveInAddBuffer(IntPtr hwi, ref WAVEHDR pwh, uint cbwh);
        [DllImport("winmm.dll")]
        private static extern int waveInStart(IntPtr hwi);
        [DllImport("winmm.dll")]
        private static extern int waveInStop(IntPtr hwi);
        [DllImport("winmm.dll")]
        private static extern int waveInReset(IntPtr hwi);
        [DllImport("winmm.dll")]
        private static extern int waveInClose(IntPtr hwi);
        [DllImport("winmm.dll", CharSet = CharSet.Unicode)]
        private static extern uint waveInGetNumDevs();
        [DllImport("winmm.dll", CharSet = CharSet.Unicode)]
        private static extern int waveInGetDevCaps(uint uDeviceID, out WAVEINCAPS pwic, uint cbwic);

        [DllImport("kernel32.dll", CharSet = CharSet.Unicode)]
        private static extern IntPtr CreateEventW(IntPtr lpEventAttributes, bool bManualReset, bool bInitialState, string lpName);
        [DllImport("kernel32.dll")]
        private static extern uint WaitForSingleObject(IntPtr hHandle, uint dwMilliseconds);
        [DllImport("kernel32.dll")]
        private static extern bool CloseHandle(IntPtr hObject);
    }
}
