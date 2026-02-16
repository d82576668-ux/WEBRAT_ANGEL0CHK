using System;
using System.Security.Cryptography;
using System.Runtime.InteropServices;

namespace WebratCs.Common
{
    public static class Crypto
    {
        public static byte[] DeriveKey(string secret)
        {
            using (var sha = SHA256.Create())
            {
                return sha.ComputeHash(System.Text.Encoding.UTF8.GetBytes(secret));
            }
        }

        public static void RandomFill(byte[] buf)
        {
            using (var rng = RandomNumberGenerator.Create())
            {
                rng.GetBytes(buf);
            }
        }

        public class AesGcmResult
        {
            public byte[] Iv;
            public byte[] Tag;
            public byte[] Ciphertext;
        }

        public static AesGcmResult EncryptAesGcm(byte[] plain, string secret)
        {
            var key = DeriveKey(secret);
            var iv = new byte[12];
            RandomFill(iv);
            var tag = new byte[16];
            var ct = new byte[plain.Length];
            if (TryEncryptAesGcmManaged(key, iv, plain, ct, tag))
            {
                return new AesGcmResult { Iv = iv, Tag = tag, Ciphertext = ct };
            }
            if (TryEncryptAesGcmCng(key, iv, plain, out ct, out tag))
            {
                return new AesGcmResult { Iv = iv, Tag = tag, Ciphertext = ct };
            }
            throw new Exception("AES-GCM not available");
        }

        public static byte[] DecryptAesGcm(byte[] ct, byte[] iv, byte[] tag, string secret)
        {
            var key = DeriveKey(secret);
            var plain = new byte[ct.Length];
            if (TryDecryptAesGcmManaged(key, iv, ct, tag, plain))
            {
                return plain;
            }
            if (TryDecryptAesGcmCng(key, iv, ct, tag, out plain))
            {
                return plain;
            }
            throw new Exception("AES-GCM not available");
        }

        private static bool TryEncryptAesGcmManaged(byte[] key, byte[] iv, byte[] plain, byte[] ct, byte[] tag)
        {
            try
            {
                var t = Type.GetType("System.Security.Cryptography.AesGcm, System.Security.Cryptography", throwOnError: false);
                if (t == null) t = Type.GetType("System.Security.Cryptography.AesGcm, System.Security.Cryptography.Algorithms", throwOnError: false);
                if (t == null) return false;
                using (var aesgcm = (IDisposable)Activator.CreateInstance(t, key))
                {
                    var encMethod = t.GetMethod("Encrypt", new Type[] { typeof(byte[]), typeof(byte[]), typeof(byte[]), typeof(byte[]), typeof(byte[]) });
                    if (encMethod == null) return false;
                    encMethod.Invoke(aesgcm, new object[] { iv, plain, ct, tag, null });
                }
                return true;
            }
            catch { return false; }
        }

        private static bool TryDecryptAesGcmManaged(byte[] key, byte[] iv, byte[] ct, byte[] tag, byte[] plain)
        {
            try
            {
                var t = Type.GetType("System.Security.Cryptography.AesGcm, System.Security.Cryptography", throwOnError: false);
                if (t == null) t = Type.GetType("System.Security.Cryptography.AesGcm, System.Security.Cryptography.Algorithms", throwOnError: false);
                if (t == null) return false;
                using (var aesgcm = (IDisposable)Activator.CreateInstance(t, key))
                {
                    var decMethod = t.GetMethod("Decrypt", new Type[] { typeof(byte[]), typeof(byte[]), typeof(byte[]), typeof(byte[]), typeof(byte[]) });
                    if (decMethod == null) return false;
                    decMethod.Invoke(aesgcm, new object[] { iv, ct, plain, tag, null });
                }
                return true;
            }
            catch { return false; }
        }

        // CNG (bcrypt) fallback
        private static bool TryEncryptAesGcmCng(byte[] key, byte[] iv, byte[] plain, out byte[] ct, out byte[] tag)
        {
            ct = new byte[plain.Length];
            tag = new byte[16];
            IntPtr hAlg = IntPtr.Zero;
            IntPtr hKey = IntPtr.Zero;
            GCHandle ivPin = default(GCHandle);
            GCHandle tagPin = default(GCHandle);
            IntPtr keyObj = IntPtr.Zero;
            try
            {
                int status = BCryptOpenAlgorithmProvider(out hAlg, BCRYPT_AES_ALG_HANDLE, null, 0);
                if (status != 0) return false;
                var modeBytes = System.Text.Encoding.Unicode.GetBytes("ChainingModeGCM\0");
                status = BCryptSetProperty(hAlg, BCRYPT_CHAINING_MODE, modeBytes, modeBytes.Length, 0);
                if (status != 0) return false;
                // allocate key object buffer
                int cbObj = 0;
                {
                    int propLen = 0;
                    var outBuf = new byte[4];
                    status = BCryptGetProperty(hAlg, BCRYPT_OBJECT_LENGTH, outBuf, outBuf.Length, out propLen, 0);
                    if (status != 0 || propLen < 4) return false;
                    cbObj = BitConverter.ToInt32(outBuf, 0);
                    if (cbObj <= 0) return false;
                    keyObj = Marshal.AllocHGlobal(cbObj);
                }
                status = BCryptGenerateSymmetricKey(hAlg, out hKey, keyObj, cbObj, key, key.Length, 0);
                if (status != 0) return false;
                ivPin = GCHandle.Alloc(iv, GCHandleType.Pinned);
                tagPin = GCHandle.Alloc(tag, GCHandleType.Pinned);
                var authInfo = new BCRYPT_AUTHENTICATED_CIPHER_MODE_INFO();
                authInfo.cbSize = Marshal.SizeOf(typeof(BCRYPT_AUTHENTICATED_CIPHER_MODE_INFO));
                authInfo.dwInfoVersion = 1;
                authInfo.pbNonce = ivPin.AddrOfPinnedObject();
                authInfo.cbNonce = iv.Length;
                authInfo.pbTag = tagPin.AddrOfPinnedObject();
                authInfo.cbTag = tag.Length;
                authInfo.cbAAD = 0;
                authInfo.cbAuthData = 0;
                authInfo.cbData = plain.Length;
                int cbResult = 0;
                status = BCryptEncrypt(hKey,
                    plain, plain.Length,
                    ref authInfo,
                    null, 0,
                    ct, ct.Length,
                    ref cbResult,
                    0);
                if (status != 0) return false;
                return true;
            }
            catch { return false; }
            finally
            {
                if (tagPin.IsAllocated) tagPin.Free();
                if (ivPin.IsAllocated) ivPin.Free();
                if (keyObj != IntPtr.Zero) { try { Marshal.FreeHGlobal(keyObj); } catch { } }
                if (hKey != IntPtr.Zero) BCryptDestroyKey(hKey);
                if (hAlg != IntPtr.Zero) BCryptCloseAlgorithmProvider(hAlg, 0);
            }
        }

        private static bool TryDecryptAesGcmCng(byte[] key, byte[] iv, byte[] ct, byte[] tag, out byte[] plain)
        {
            plain = new byte[ct.Length];
            IntPtr hAlg = IntPtr.Zero;
            IntPtr hKey = IntPtr.Zero;
            GCHandle ivPin = default(GCHandle);
            GCHandle tagPin = default(GCHandle);
            IntPtr keyObj = IntPtr.Zero;
            try
            {
                int status = BCryptOpenAlgorithmProvider(out hAlg, BCRYPT_AES_ALG_HANDLE, null, 0);
                if (status != 0) return false;
                var modeBytes = System.Text.Encoding.Unicode.GetBytes("ChainingModeGCM\0");
                status = BCryptSetProperty(hAlg, BCRYPT_CHAINING_MODE, modeBytes, modeBytes.Length, 0);
                if (status != 0) return false;
                // allocate key object buffer
                int cbObj = 0;
                {
                    int propLen = 0;
                    var outBuf = new byte[4];
                    status = BCryptGetProperty(hAlg, BCRYPT_OBJECT_LENGTH, outBuf, outBuf.Length, out propLen, 0);
                    if (status != 0 || propLen < 4) return false;
                    cbObj = BitConverter.ToInt32(outBuf, 0);
                    if (cbObj <= 0) return false;
                    keyObj = Marshal.AllocHGlobal(cbObj);
                }
                status = BCryptGenerateSymmetricKey(hAlg, out hKey, keyObj, cbObj, key, key.Length, 0);
                if (status != 0) return false;
                ivPin = GCHandle.Alloc(iv, GCHandleType.Pinned);
                tagPin = GCHandle.Alloc(tag, GCHandleType.Pinned);
                var authInfo = new BCRYPT_AUTHENTICATED_CIPHER_MODE_INFO();
                authInfo.cbSize = Marshal.SizeOf(typeof(BCRYPT_AUTHENTICATED_CIPHER_MODE_INFO));
                authInfo.dwInfoVersion = 1;
                authInfo.pbNonce = ivPin.AddrOfPinnedObject();
                authInfo.cbNonce = iv.Length;
                authInfo.pbTag = tagPin.AddrOfPinnedObject();
                authInfo.cbTag = tag.Length;
                authInfo.cbAAD = 0;
                authInfo.cbAuthData = 0;
                authInfo.cbData = ct.Length;
                int cbResult = 0;
                status = BCryptDecrypt(hKey,
                    ct, ct.Length,
                    ref authInfo,
                    null, 0,
                    plain, plain.Length,
                    ref cbResult,
                    0);
                if (status != 0) return false;
                return true;
            }
            catch { return false; }
            finally
            {
                if (tagPin.IsAllocated) tagPin.Free();
                if (ivPin.IsAllocated) ivPin.Free();
                if (keyObj != IntPtr.Zero) { try { Marshal.FreeHGlobal(keyObj); } catch { } }
                if (hKey != IntPtr.Zero) BCryptDestroyKey(hKey);
                if (hAlg != IntPtr.Zero) BCryptCloseAlgorithmProvider(hAlg, 0);
            }
        }

        private const string BCRYPT_AES_ALG_HANDLE = "AES";
        private const string BCRYPT_CHAINING_MODE = "ChainingMode";
        private const string BCRYPT_OBJECT_LENGTH = "ObjectLength";

        [StructLayout(LayoutKind.Sequential)]
        private struct BCRYPT_AUTHENTICATED_CIPHER_MODE_INFO
        {
            public int cbSize;
            public int dwInfoVersion;
            public IntPtr pbNonce;
            public int cbNonce;
            public IntPtr pbAuthData;
            public int cbAuthData;
            public IntPtr pbTag;
            public int cbTag;
            public IntPtr pbMacContext;
            public int cbMacContext;
            public int cbAAD;
            public int cbData;
            public int dwFlags;
        }

        [DllImport("bcrypt.dll", CharSet = CharSet.Unicode)]
        private static extern int BCryptOpenAlgorithmProvider(out IntPtr phAlgorithm, string pszAlgId, string pszImplementation, int dwFlags);

        [DllImport("bcrypt.dll", CharSet = CharSet.Unicode)]
        private static extern int BCryptSetProperty(IntPtr hObject, string pszProperty, byte[] pbInput, int cbInput, int dwFlags);
        [DllImport("bcrypt.dll", CharSet = CharSet.Unicode)]
        private static extern int BCryptGetProperty(IntPtr hObject, string pszProperty, byte[] pbOutput, int cbOutput, out int pcbResult, int dwFlags);

        [DllImport("bcrypt.dll")]
        private static extern int BCryptGenerateSymmetricKey(IntPtr hAlgorithm, out IntPtr phKey, IntPtr pbKeyObject, int cbKeyObject, byte[] pbSecret, int cbSecret, int dwFlags);

        [DllImport("bcrypt.dll")]
        private static extern int BCryptEncrypt(IntPtr hKey, byte[] pbInput, int cbInput, ref BCRYPT_AUTHENTICATED_CIPHER_MODE_INFO pPaddingInfo, byte[] pbIV, int cbIV, byte[] pbOutput, int cbOutput, ref int pcbResult, int dwFlags);

        [DllImport("bcrypt.dll")]
        private static extern int BCryptDecrypt(IntPtr hKey, byte[] pbInput, int cbInput, ref BCRYPT_AUTHENTICATED_CIPHER_MODE_INFO pPaddingInfo, byte[] pbIV, int cbIV, byte[] pbOutput, int cbOutput, ref int pcbResult, int dwFlags);

        [DllImport("bcrypt.dll")]
        private static extern int BCryptDestroyKey(IntPtr hKey);

        [DllImport("bcrypt.dll")]
        private static extern int BCryptCloseAlgorithmProvider(IntPtr hAlgorithm, int dwFlags);
    }
}
