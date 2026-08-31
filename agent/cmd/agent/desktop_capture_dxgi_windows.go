//go:build windows && cgo

package main

/*
#cgo windows CFLAGS: -O3
#cgo windows LDFLAGS: -ld3d11 -ldxgi -ldxguid -lole32
#define COBJMACROS
#include <windows.h>
#include <d3d11.h>
#include <dxgi1_2.h>
#include <stdlib.h>
#include <stdint.h>
#include <string.h>

// Scale a packed BGRA desktop surface without crossing the cgo boundary per
// pixel.  The previous equivalent Go loop was deliberately high quality, but
// needed about 290 ms for 2560x1920 -> 1920x1440 on the production Windows
// VM.  Keeping the fixed-point bilinear math in one optimized native loop
// preserves small-text readability while making 30 FPS attainable.
static int remoteit_scale_bgra_bilinear(
    const unsigned char *source, int source_width, int source_height,
    unsigned char *target, int target_width, int target_height,
    const int32_t *scale_x, const int32_t *scale_weight) {
    if (!source || !target || !scale_x || !scale_weight ||
        source_width <= 0 || source_height <= 0 ||
        target_width <= 0 || target_height <= 0) return 0;

    const size_t source_stride = (size_t)source_width * 4u;
    const size_t target_stride = (size_t)target_width * 4u;
    for (int target_y = 0; target_y < target_height; ++target_y) {
        int64_t source_y_256 = ((int64_t)(target_y * 2 + 1) * source_height * 128) / target_height - 128;
        if (source_y_256 < 0) source_y_256 = 0;
        int64_t maximum_y = (int64_t)(source_height - 1) * 256;
        if (source_y_256 > maximum_y) source_y_256 = maximum_y;
        int source_y = (int)(source_y_256 >> 8);
        int weight_y = (int)(source_y_256 & 255);
        if (source_y >= source_height - 1) {
            source_y = source_height - 1;
            weight_y = 0;
        }
        int next_y = source_y + (source_y + 1 < source_height);
        const unsigned char *top_row = source + (size_t)source_y * source_stride;
        const unsigned char *bottom_row = source + (size_t)next_y * source_stride;
        unsigned char *target_row = target + (size_t)target_y * target_stride;
        const int inverse_y = 256 - weight_y;

        for (int target_x = 0; target_x < target_width; ++target_x) {
            int source_x = scale_x[target_x];
            int next_x = source_x + (source_x + 1 < source_width);
            int weight_x = scale_weight[target_x];
            int inverse_x = 256 - weight_x;
            const unsigned char *top_left = top_row + (size_t)source_x * 4u;
            const unsigned char *top_right = top_row + (size_t)next_x * 4u;
            const unsigned char *bottom_left = bottom_row + (size_t)source_x * 4u;
            const unsigned char *bottom_right = bottom_row + (size_t)next_x * 4u;
            unsigned char *pixel = target_row + (size_t)target_x * 4u;

            // Unrolling the four BGRA channels lets GCC vectorise the hot
            // loop and removes the innermost branch/counter completely.
            #define REMOTEIT_SCALE_CHANNEL(channel) do { \
                int top = top_left[channel] * inverse_x + top_right[channel] * weight_x; \
                int bottom = bottom_left[channel] * inverse_x + bottom_right[channel] * weight_x; \
                pixel[channel] = (unsigned char)((top * inverse_y + bottom * weight_y + 32768) >> 16); \
            } while (0)
            REMOTEIT_SCALE_CHANNEL(0);
            REMOTEIT_SCALE_CHANNEL(1);
            REMOTEIT_SCALE_CHANNEL(2);
            REMOTEIT_SCALE_CHANNEL(3);
            #undef REMOTEIT_SCALE_CHANNEL
        }
    }
    return 1;
}

// The explicit 60 FPS mode has a 16.7 ms end-to-end frame budget. On a
// 2560x1920 VMware desktop the high-quality bilinear pass alone consumed
// 12-14 ms, although DXGI capture and TurboJPEG together needed only another
// 11-12 ms. During active motion use a point sampler with centre-of-pixel
// coordinates. It is allocation-free, preserves hard UI edges (rather than
// blurring glyphs), and the normal bilinear/native-resolution sharp frame is
// restored as soon as input stops.
static int remoteit_scale_bgra_realtime(
    const unsigned char *source, int source_width, int source_height,
    unsigned char *target, int target_width, int target_height,
    const int32_t *scale_x, const int32_t *scale_weight) {
    if (!source || !target || !scale_x || !scale_weight ||
        source_width <= 0 || source_height <= 0 ||
        target_width <= 0 || target_height <= 0) return 0;

    const size_t source_stride = (size_t)source_width * 4u;
    const size_t target_stride = (size_t)target_width * 4u;
    for (int target_y = 0; target_y < target_height; ++target_y) {
        int source_y = (int)(((int64_t)(target_y * 2 + 1) * source_height) / (target_height * 2));
        if (source_y >= source_height) source_y = source_height - 1;
        const unsigned char *source_row = source + (size_t)source_y * source_stride;
        unsigned char *target_row = target + (size_t)target_y * target_stride;
        for (int target_x = 0; target_x < target_width; ++target_x) {
            int source_x = scale_x[target_x] + (scale_weight[target_x] >= 128);
            if (source_x >= source_width) source_x = source_width - 1;
            uint32_t pixel;
            memcpy(&pixel, source_row + (size_t)source_x * 4u, sizeof(pixel));
            memcpy(target_row + (size_t)target_x * 4u, &pixel, sizeof(pixel));
        }
    }
    return 1;
}

typedef struct remoteit_dxgi_capture {
    ID3D11Device *device;
    ID3D11DeviceContext *context;
    IDXGIOutputDuplication *duplication;
    ID3D11Texture2D *staging;
    int width;
    int height;
    int has_frame;
    int polled;
} remoteit_dxgi_capture;

static void remoteit_dxgi_destroy(remoteit_dxgi_capture *capture) {
    if (!capture) return;
    if (capture->staging) ID3D11Texture2D_Release(capture->staging);
    if (capture->duplication) IDXGIOutputDuplication_Release(capture->duplication);
    if (capture->context) ID3D11DeviceContext_Release(capture->context);
    if (capture->device) ID3D11Device_Release(capture->device);
    free(capture);
}

static remoteit_dxgi_capture *remoteit_dxgi_create(int expected_width, int expected_height, int expected_x, int expected_y, int *width, int *height, int *failure_stage, long long *failure_code) {
    remoteit_dxgi_capture *capture = (remoteit_dxgi_capture *)calloc(1, sizeof(remoteit_dxgi_capture));
    if (failure_stage) *failure_stage = 0;
    if (failure_code) *failure_code = 0;
    if (!capture) {
        if (failure_stage) *failure_stage = 1;
        if (failure_code) *failure_code = (long long)E_OUTOFMEMORY;
        return NULL;
    }

    IDXGIFactory1 *factory = NULL;
    HRESULT result = CreateDXGIFactory1(&IID_IDXGIFactory1, (void **)&factory);
    int stage = 2;
    DXGI_OUTPUT_DESC output_desc;
    memset(&output_desc, 0, sizeof(output_desc));
    if (SUCCEEDED(result)) {
        result = DXGI_ERROR_NOT_FOUND;
        for (UINT adapter_index = 0; ; ++adapter_index) {
            IDXGIAdapter1 *adapter = NULL;
            stage = 3;
            HRESULT adapter_result = IDXGIFactory1_EnumAdapters1(factory, adapter_index, &adapter);
            if (adapter_result == DXGI_ERROR_NOT_FOUND) break;
            if (FAILED(adapter_result) || !adapter) {
                result = adapter_result;
                continue;
            }

            ID3D11Device *device = NULL;
            ID3D11DeviceContext *context = NULL;
            D3D_FEATURE_LEVEL feature_level;
            stage = 4;
            HRESULT device_result = D3D11CreateDevice(
                (IDXGIAdapter *)adapter, D3D_DRIVER_TYPE_UNKNOWN, NULL,
                D3D11_CREATE_DEVICE_BGRA_SUPPORT, NULL, 0, D3D11_SDK_VERSION,
                &device, &feature_level, &context);
            if (FAILED(device_result)) {
                result = device_result;
                IDXGIAdapter1_Release(adapter);
                continue;
            }

            for (UINT output_index = 0; ; ++output_index) {
                IDXGIOutput *output = NULL;
                stage = 5;
                HRESULT output_result = IDXGIAdapter1_EnumOutputs(adapter, output_index, &output);
                if (output_result == DXGI_ERROR_NOT_FOUND) break;
                if (FAILED(output_result) || !output) {
                    result = output_result;
                    continue;
                }
                DXGI_OUTPUT_DESC candidate_desc;
                memset(&candidate_desc, 0, sizeof(candidate_desc));
                stage = 6;
                output_result = IDXGIOutput_GetDesc(output, &candidate_desc);
                int candidate_width = candidate_desc.DesktopCoordinates.right - candidate_desc.DesktopCoordinates.left;
                int candidate_height = candidate_desc.DesktopCoordinates.bottom - candidate_desc.DesktopCoordinates.top;
                if (FAILED(output_result) || !candidate_desc.AttachedToDesktop ||
                    candidate_width != expected_width || candidate_height != expected_height ||
                    candidate_desc.DesktopCoordinates.left != expected_x || candidate_desc.DesktopCoordinates.top != expected_y) {
                    if (FAILED(output_result)) result = output_result;
                    IDXGIOutput_Release(output);
                    continue;
                }

                IDXGIOutput1 *output1 = NULL;
                stage = 7;
                output_result = IDXGIOutput_QueryInterface(output, &IID_IDXGIOutput1, (void **)&output1);
                if (SUCCEEDED(output_result)) {
                    stage = 8;
                    output_result = IDXGIOutput1_DuplicateOutput(output1, (IUnknown *)device, &capture->duplication);
                }
                if (output1) IDXGIOutput1_Release(output1);
                IDXGIOutput_Release(output);
                result = output_result;
                if (SUCCEEDED(output_result) && capture->duplication) {
                    capture->device = device;
                    capture->context = context;
                    output_desc = candidate_desc;
                    device = NULL;
                    context = NULL;
                    break;
                }
            }

            if (context) ID3D11DeviceContext_Release(context);
            if (device) ID3D11Device_Release(device);
            IDXGIAdapter1_Release(adapter);
            if (capture->duplication) break;
        }
    }
    if (factory) IDXGIFactory1_Release(factory);

    if (FAILED(result) || !capture->duplication) {
        if (failure_stage) *failure_stage = stage;
        if (failure_code) *failure_code = (long long)result;
        remoteit_dxgi_destroy(capture);
        return NULL;
    }
    capture->width = output_desc.DesktopCoordinates.right - output_desc.DesktopCoordinates.left;
    capture->height = output_desc.DesktopCoordinates.bottom - output_desc.DesktopCoordinates.top;
    if (capture->width <= 0 || capture->height <= 0) {
        if (failure_stage) *failure_stage = 9;
        if (failure_code) *failure_code = (long long)E_FAIL;
        remoteit_dxgi_destroy(capture);
        return NULL;
    }
    *width = capture->width;
    *height = capture->height;
    return capture;
}

// 1 means that a new frame was copied, 0 means that the desktop did not change
// before the bounded wait expired, and -1 means that the duplication interface
// should be recreated.
static int remoteit_dxgi_next(remoteit_dxgi_capture *capture, unsigned char *destination, size_t destination_size, int *failure_stage, long long *failure_code) {
    if (failure_stage) *failure_stage = 0;
    if (failure_code) *failure_code = 0;
    if (!capture || !destination || destination_size < (size_t)capture->width * (size_t)capture->height * 4u) {
        if (failure_stage) *failure_stage = 10;
        if (failure_code) *failure_code = (long long)E_INVALIDARG;
        return -1;
    }

    DXGI_OUTDUPL_FRAME_INFO frame_info;
    IDXGIResource *resource = NULL;
    memset(&frame_info, 0, sizeof(frame_info));
    // The first frame is not guaranteed to be ready immediately after
    // DuplicateOutput.  VMware and some RDP display drivers also publish the
    // next duplicated frame a few milliseconds after DWM presents it.  A
    // zero-timeout polling loop can repeatedly miss that narrow window and
    // incorrectly force every capture through the much slower GDI fallback.
    // The short steady-state wait stays below one 60 FPS interval and is only
    // paid when there is no frame ready yet.
    // The Go cadence already sleeps until the selected FPS deadline. A second
    // eight-millisecond wait here was paid on almost every cursor-only frame
    // because Desktop Duplication exposes the pointer independently from the
    // desktop texture. Poll the established duplication immediately; cursor
    // composition still provides the fresh frame. Keep the generous bootstrap
    // wait only for the first frame after a desktop/device transition.
    UINT timeout_ms = capture->polled ? 0u : 250u;
    HRESULT result = IDXGIOutputDuplication_AcquireNextFrame(capture->duplication, timeout_ms, &frame_info, &resource);
    capture->polled = 1;
    if (result == DXGI_ERROR_WAIT_TIMEOUT) return 0;
    if (FAILED(result)) {
        if (failure_stage) *failure_stage = 11;
        if (failure_code) *failure_code = (long long)result;
        return -1;
    }

    ID3D11Texture2D *desktop_texture = NULL;
    result = IDXGIResource_QueryInterface(resource, &IID_ID3D11Texture2D, (void **)&desktop_texture);
    if (SUCCEEDED(result)) {
        D3D11_TEXTURE2D_DESC description;
        ID3D11Texture2D_GetDesc(desktop_texture, &description);
        if ((int)description.Width != capture->width || (int)description.Height != capture->height) {
            result = E_FAIL;
        } else if (!capture->staging) {
            description.Usage = D3D11_USAGE_STAGING;
            description.BindFlags = 0;
            description.CPUAccessFlags = D3D11_CPU_ACCESS_READ;
            description.MiscFlags = 0;
            result = ID3D11Device_CreateTexture2D(capture->device, &description, NULL, &capture->staging);
        }
    }
    if (SUCCEEDED(result)) {
        ID3D11DeviceContext_CopyResource(capture->context, (ID3D11Resource *)capture->staging, (ID3D11Resource *)desktop_texture);
        D3D11_MAPPED_SUBRESOURCE mapped;
        memset(&mapped, 0, sizeof(mapped));
        result = ID3D11DeviceContext_Map(capture->context, (ID3D11Resource *)capture->staging, 0, D3D11_MAP_READ, 0, &mapped);
        if (SUCCEEDED(result)) {
            size_t row_size = (size_t)capture->width * 4u;
            for (int row = 0; row < capture->height; row++) {
                memcpy(destination + (size_t)row * row_size, (unsigned char *)mapped.pData + (size_t)row * mapped.RowPitch, row_size);
            }
            ID3D11DeviceContext_Unmap(capture->context, (ID3D11Resource *)capture->staging, 0);
            capture->has_frame = 1;
        }
    }

    if (desktop_texture) ID3D11Texture2D_Release(desktop_texture);
    if (resource) IDXGIResource_Release(resource);
    IDXGIOutputDuplication_ReleaseFrame(capture->duplication);
    if (FAILED(result)) {
        if (failure_stage) *failure_stage = 12;
        if (failure_code) *failure_code = (long long)result;
    }
    return SUCCEEDED(result) ? 1 : -1;
}
*/
import "C"

import (
	"fmt"
	"time"
	"unsafe"
)

type desktopFastCapturer struct {
	handle     *C.remoteit_dxgi_capture
	width      int
	height     int
	retryAfter time.Time
	detail     string
}

func (capturer *desktopFastCapturer) Close() {
	if capturer.handle != nil {
		C.remoteit_dxgi_destroy(capturer.handle)
	}
	detail := capturer.detail
	*capturer = desktopFastCapturer{detail: detail}
}

// CaptureBGRA returns 1 for a usable DXGI frame, 0 when DXGI is healthy but
// the desktop has not changed, and -1 while the fast backend is unavailable.
func (capturer *desktopFastCapturer) CaptureBGRA(pixels []byte, x, y, width, height int) int {
	required := width * height * 4
	if width <= 0 || height <= 0 || required <= 0 || len(pixels) < required {
		capturer.detail = fmt.Sprintf("gdi-dxgi-buffer-%d-%d-%d", width, height, len(pixels))
		return -1
	}
	if time.Now().Before(capturer.retryAfter) {
		if capturer.detail == "" {
			capturer.detail = "gdi-dxgi-retry"
		}
		return -1
	}
	if capturer.handle == nil {
		var outputWidth, outputHeight, failureStage C.int
		var failureCode C.longlong
		capturer.handle = C.remoteit_dxgi_create(C.int(width), C.int(height), C.int(x), C.int(y), &outputWidth, &outputHeight, &failureStage, &failureCode)
		capturer.width, capturer.height = int(outputWidth), int(outputHeight)
		if capturer.handle == nil || capturer.width != width || capturer.height != height {
			if capturer.handle == nil {
				capturer.detail = fmt.Sprintf("gdi-dxgi-create-%d-%08x", int(failureStage), uint32(failureCode))
			} else {
				capturer.detail = fmt.Sprintf("gdi-dxgi-size-%dx%d", capturer.width, capturer.height)
			}
			capturer.Close()
			capturer.retryAfter = time.Now().Add(30 * time.Second)
			return -1
		}
	}
	var failureStage C.int
	var failureCode C.longlong
	result := int(C.remoteit_dxgi_next(
		capturer.handle,
		(*C.uchar)(unsafe.Pointer(&pixels[0])),
		C.size_t(len(pixels)),
		&failureStage,
		&failureCode,
	))
	if result < 0 {
		capturer.detail = fmt.Sprintf("gdi-dxgi-frame-%d-%08x", int(failureStage), uint32(failureCode))
		capturer.Close()
		capturer.retryAfter = time.Now().Add(time.Second)
		return -1
	}
	if result > 0 {
		capturer.detail = "dxgi"
	}
	return result
}

func (capturer *desktopFastCapturer) BackendDetail() string {
	if capturer.detail == "" {
		return "gdi-dxgi-wait"
	}
	return capturer.detail
}

func scaleDesktopBGRAFast(source []byte, sourceWidth, sourceHeight int, target []byte, targetWidth, targetHeight int, scaleX, scaleWeight []int32) bool {
	if sourceWidth <= 0 || sourceHeight <= 0 || targetWidth <= 0 || targetHeight <= 0 ||
		len(source) < sourceWidth*sourceHeight*4 || len(target) < targetWidth*targetHeight*4 ||
		len(scaleX) < targetWidth || len(scaleWeight) < targetWidth {
		return false
	}
	return C.remoteit_scale_bgra_bilinear(
		(*C.uchar)(unsafe.Pointer(&source[0])), C.int(sourceWidth), C.int(sourceHeight),
		(*C.uchar)(unsafe.Pointer(&target[0])), C.int(targetWidth), C.int(targetHeight),
		(*C.int32_t)(unsafe.Pointer(&scaleX[0])), (*C.int32_t)(unsafe.Pointer(&scaleWeight[0])),
	) != 0
}

func scaleDesktopBGRARealtime(source []byte, sourceWidth, sourceHeight int, target []byte, targetWidth, targetHeight int, scaleX, scaleWeight []int32) bool {
	if sourceWidth <= 0 || sourceHeight <= 0 || targetWidth <= 0 || targetHeight <= 0 ||
		len(source) < sourceWidth*sourceHeight*4 || len(target) < targetWidth*targetHeight*4 ||
		len(scaleX) < targetWidth || len(scaleWeight) < targetWidth {
		return false
	}
	return C.remoteit_scale_bgra_realtime(
		(*C.uchar)(unsafe.Pointer(&source[0])), C.int(sourceWidth), C.int(sourceHeight),
		(*C.uchar)(unsafe.Pointer(&target[0])), C.int(targetWidth), C.int(targetHeight),
		(*C.int32_t)(unsafe.Pointer(&scaleX[0])), (*C.int32_t)(unsafe.Pointer(&scaleWeight[0])),
	) != 0
}
