using System;
using System.Drawing;
using System.Drawing.Imaging;
using System.Runtime.InteropServices;
using System.Windows.Forms;
using WebratCs.Camera;

namespace WebratCs.CameraTest
{
    static class Program
    {
        [STAThread]
        static void Main(string[] args)
        {
            string deviceName = args != null && args.Length > 0 ? args[0] : null;
            int w = 640, h = 480, fps = 10, quality = 80;
            string outPath = System.IO.Path.Combine(AppDomain.CurrentDomain.BaseDirectory, "test-camera.jpg");
            Application.EnableVisualStyles();
            Application.SetCompatibleTextRenderingDefault(false);
            var form = new CamForm(deviceName, w, h, fps, quality, outPath);
            Application.Run(form);
        }

        class CamForm : Form
        {
            string dev;
            int w, h, fps, quality;
            string outPath;
            IntPtr hwnd = IntPtr.Zero;
            int idx = 0;
            public CamForm(string deviceName, int w, int h, int fps, int quality, string outPath)
            {
                this.dev = deviceName;
                this.w = w; this.h = h; this.fps = fps; this.quality = quality; this.outPath = outPath;
                this.Width = w; this.Height = h;
                this.ShowInTaskbar = false;
                this.Text = "CamTest";
                this.FormBorderStyle = FormBorderStyle.FixedToolWindow;
                this.StartPosition = FormStartPosition.CenterScreen;
            }
            protected override void OnShown(EventArgs e)
            {
                base.OnShown(e);
                try
                {
                    var list = AvicapCamera.ListDevices();
                    if (list == null || list.Length == 0) { Console.WriteLine("NO_CAMERAS"); Close(); return; }
                    if (string.IsNullOrEmpty(dev)) dev = list[0];
                    idx = FindDeviceIndex(dev);
                    hwnd = capCreateCaptureWindowA("cap", 0x40000000 /*WS_CHILD*/, 0, 0, w, h, this.Handle, 0);
                    if (hwnd == IntPtr.Zero) { Console.WriteLine("CREATE_FAILED"); Close(); return; }
                    if (SendMessage(hwnd, WM_CAP_DRIVER_CONNECT, (IntPtr)idx, IntPtr.Zero) == IntPtr.Zero)
                    { Console.WriteLine("CONNECT_FAILED"); Close(); return; }
                    SendMessage(hwnd, WM_CAP_SET_SCALE, (IntPtr)1, IntPtr.Zero);
                    SendMessage(hwnd, WM_CAP_SET_PREVIEW, (IntPtr)1, IntPtr.Zero);
                    SendMessage(hwnd, WM_CAP_SET_PREVIEWRATE, (IntPtr)Math.Max(50, 1000 / fps), IntPtr.Zero);
                    var t = new Timer();
                    t.Interval = 300;
                    t.Tick += (s, ev) =>
                    {
                        t.Stop();
                        try
                        {
                            SendMessage(hwnd, WM_CAP_GRAB_FRAME, IntPtr.Zero, IntPtr.Zero);
                            string bmpPath = System.IO.Path.Combine(AppDomain.CurrentDomain.BaseDirectory, "test-camera.bmp");
                            IntPtr strPtr = Marshal.StringToHGlobalAnsi(bmpPath);
                            SendMessage(hwnd, WM_CAP_FILE_SAVEDIB, IntPtr.Zero, strPtr);
                            Marshal.FreeHGlobal(strPtr);
                            Image img = null;
                            if (System.IO.File.Exists(bmpPath))
                            {
                                using (var dib = Image.FromFile(bmpPath))
                                {
                                    img = new Bitmap(dib);
                                }
                                try { System.IO.File.Delete(bmpPath); } catch { }
                            }
                            if (img == null)
                            {
                                SendMessage(hwnd, WM_CAP_EDIT_COPY, IntPtr.Zero, IntPtr.Zero);
                                try { img = Clipboard.GetImage(); } catch { img = null; }
                            }
                            if (img == null)
                            {
                                img = CaptureWindowBitmap(hwnd);
                            }
                            if (img == null) { Console.WriteLine("CAPTURE_FAILED"); }
                            else
                            {
                                using (img)
                                using (var ms = new System.IO.MemoryStream())
                                {
                                    var enc = GetJpegEncoder();
                                    var ep = new EncoderParameters(1);
                                    ep.Param[0] = new EncoderParameter(System.Drawing.Imaging.Encoder.Quality, quality);
                                    img.Save(ms, enc, ep);
                                    System.IO.File.WriteAllBytes(outPath, ms.ToArray());
                                }
                                using (var bmp = new Bitmap(outPath))
                                {
                                    bool isBlack = IsMostlyBlack(bmp);
                                    Console.WriteLine(isBlack ? ("CAPTURE_BLACK " + outPath) : ("CAPTURE_OK " + outPath));
                                }
                            }
                        }
                        catch (Exception ex) { Console.WriteLine("ERROR " + ex.Message); }
                        finally
                        {
                            try { SendMessage(hwnd, WM_CAP_DRIVER_DISCONNECT, IntPtr.Zero, IntPtr.Zero); } catch { }
                            Close();
                        }
                    };
                    t.Start();
                }
                catch (Exception ex) { Console.WriteLine("ERROR " + ex.Message); Close(); }
            }
        }
        static void Cleanup(IntPtr hwnd, int idx)
        {
            try { SendMessage(hwnd, WM_CAP_DRIVER_DISCONNECT, IntPtr.Zero, IntPtr.Zero); } catch { }
            try { DestroyWindow(hwnd); } catch { }
        }

        static bool IsMostlyBlack(Bitmap bmp)
        {
            int w = bmp.Width, h = bmp.Height;
            long total = 0;
            long nonBlack = 0;
            for (int y = 0; y < h; y += Math.Max(1, h / 200))
            {
                for (int x = 0; x < w; x += Math.Max(1, w / 200))
                {
                    var c = bmp.GetPixel(x, y);
                    int lum = (c.R + c.G + c.B) / 3;
                    total++;
                    if (lum > 8) nonBlack++;
                }
            }
            double ratio = (double)nonBlack / Math.Max(1, total);
            return ratio < 0.02;
        }

        static Image CaptureWindowBitmap(IntPtr hwnd)
        {
            RECT rc;
            if (!GetClientRect(hwnd, out rc)) return null;
            int w = rc.right - rc.left;
            int h = rc.bottom - rc.top;
            IntPtr hdc = GetDC(hwnd);
            if (hdc == IntPtr.Zero) return null;
            IntPtr memDC = CreateCompatibleDC(hdc);
            if (memDC == IntPtr.Zero) { ReleaseDC(hwnd, hdc); return null; }
            IntPtr hbm = CreateCompatibleBitmap(hdc, w, h);
            if (hbm == IntPtr.Zero) { DeleteDC(memDC); ReleaseDC(hwnd, hdc); return null; }
            IntPtr prev = SelectObject(memDC, hbm);
            SendMessage(hwnd, WM_CAP_GRAB_FRAME, IntPtr.Zero, IntPtr.Zero);
            IntPtr pr = PrintWindow(hwnd, memDC, (IntPtr)PW_RENDERFULLCONTENT);
            if (pr == IntPtr.Zero)
            {
                BitBlt(memDC, 0, 0, w, h, hdc, 0, 0, SRCCOPY);
            }
            Image bmp = Image.FromHbitmap(hbm);
            SelectObject(memDC, prev);
            DeleteObject(hbm);
            DeleteDC(memDC);
            ReleaseDC(hwnd, hdc);
            return bmp;
        }

        static int FindDeviceIndex(string name)
        {
            for (int i = 0; i < 20; i++)
            {
                var nameBuf = new byte[256];
                var verBuf = new byte[256];
                var ok = capGetDriverDescriptionA(i, nameBuf, nameBuf.Length, verBuf, verBuf.Length);
                if (ok)
                {
                    int n = Array.IndexOf(nameBuf, (byte)0);
                    if (n < 0) n = nameBuf.Length;
                    var nm = System.Text.Encoding.ASCII.GetString(nameBuf, 0, n).Trim();
                    if (string.Equals(nm, name ?? "", StringComparison.OrdinalIgnoreCase)) return i;
                }
            }
            return 0;
        }

        private static ImageCodecInfo GetJpegEncoder()
        {
            foreach (var c in ImageCodecInfo.GetImageEncoders())
            {
                if (c.MimeType.Equals("image/jpeg", StringComparison.OrdinalIgnoreCase)) return c;
            }
            return ImageCodecInfo.GetImageEncoders()[0];
        }

        // PInvoke
        private const int WM_CAP_START = 0x400;
        private static readonly int WM_CAP_DRIVER_CONNECT = WM_CAP_START + 10;
        private static readonly int WM_CAP_DRIVER_DISCONNECT = WM_CAP_START + 11;
        private static readonly int WM_CAP_SET_PREVIEW = WM_CAP_START + 50;
        private static readonly int WM_CAP_SET_PREVIEWRATE = WM_CAP_START + 52;
        private static readonly int WM_CAP_SET_SCALE = WM_CAP_START + 53;
        private static readonly int WM_CAP_GRAB_FRAME = WM_CAP_START + 60;
        private static readonly int WM_CAP_EDIT_COPY = WM_CAP_START + 30;
        private static readonly int WM_CAP_FILE_SAVEDIB = WM_CAP_START + 25;
        private static readonly int SRCCOPY = 0x00CC0020;
        private static readonly int PW_RENDERFULLCONTENT = 0x00000002;

        [StructLayout(LayoutKind.Sequential)]
        private struct RECT { public int left, top, right, bottom; }

        [DllImport("avicap32.dll", CharSet = CharSet.Ansi)]
        private static extern IntPtr capCreateCaptureWindowA(string lpszName, int dwStyle, int X, int Y, int nWidth, int nHeight, IntPtr hwndParent, int nID);

        [DllImport("avicap32.dll", CharSet = CharSet.Ansi)]
        private static extern bool capGetDriverDescriptionA(int wDriverIndex, byte[] lpszName, int cbName, byte[] lpszVer, int cbVer);

        [DllImport("user32.dll")]
        private static extern IntPtr SendMessage(IntPtr hWnd, int Msg, IntPtr wParam, IntPtr lParam);

        [DllImport("user32.dll")]
        private static extern bool DestroyWindow(IntPtr hWnd);

        [DllImport("user32.dll")]
        private static extern IntPtr GetDC(IntPtr hWnd);

        [DllImport("user32.dll")]
        private static extern int ReleaseDC(IntPtr hWnd, IntPtr hDC);

        [DllImport("user32.dll")]
        private static extern bool GetClientRect(IntPtr hWnd, out RECT lpRect);

        [DllImport("user32.dll")]
        private static extern IntPtr PrintWindow(IntPtr hwnd, IntPtr hdcBlt, IntPtr nFlags);

        [DllImport("gdi32.dll")]
        private static extern IntPtr CreateCompatibleDC(IntPtr hdc);

        [DllImport("gdi32.dll")]
        private static extern bool DeleteDC(IntPtr hdc);

        [DllImport("gdi32.dll")]
        private static extern IntPtr CreateCompatibleBitmap(IntPtr hdc, int cx, int cy);

        [DllImport("gdi32.dll")]
        private static extern IntPtr SelectObject(IntPtr hdc, IntPtr h);

        [DllImport("gdi32.dll")]
        private static extern bool BitBlt(IntPtr hdc, int x, int y, int cx, int cy, IntPtr hdcSrc, int x1, int y1, int rop);

        [DllImport("gdi32.dll")]
        private static extern bool DeleteObject(IntPtr hObject);
    }
}
