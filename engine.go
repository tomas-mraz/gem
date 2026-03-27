package gem

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"time"
	"unsafe"

	"github.com/go-gl/glfw/v3.3/glfw"
	"github.com/tomas-mraz/gem/component"
	"github.com/tomas-mraz/input"
	vk "github.com/tomas-mraz/vulkan"
	ash "github.com/tomas-mraz/vulkan-ash"
)

type Config struct {
	Title      string
	Width      int
	Height     int
	RayTracing bool
}

type pushConstants struct {
	OffsetX, OffsetY       float32
	Angle, Aspect          float32
	ColorR, ColorG, ColorB float32
	Brightness             float32
}

var pushConstSize = uint32(unsafe.Sizeof(pushConstants{}))

type Engine struct {
	Config Config
	Input  *input.Input
	Window *glfw.Window

	vo        ash.Vulkan
	swapchain ash.VulkanSwapchainInfo
	renderer  ash.VulkanRenderInfo
	fence     vk.Fence
	semaphore vk.Semaphore

	elapsed  float64
	dt       float64
	lastTime time.Time

	cmd      vk.CommandBuffer
	frameIdx uint32
	inFrame  bool

	vertShaderData map[string]*[]byte
	fragShaderData map[string]*[]byte

	activeRaster *rasterState
}

func New(cfg Config) *Engine {
	if cfg.Width == 0 {
		cfg.Width = 800
	}
	if cfg.Height == 0 {
		cfg.Height = 600
	}
	if cfg.Title == "" {
		cfg.Title = "gem"
	}

	runtime.LockOSThread()

	if err := glfw.Init(); err != nil {
		panic(fmt.Errorf("gem: glfw.Init failed: %w", err))
	}

	vk.SetGetInstanceProcAddr(glfw.GetVulkanGetInstanceProcAddress())
	if err := vk.Init(); err != nil {
		panic(err)
	}

	glfw.WindowHint(glfw.ClientAPI, glfw.NoAPI)
	glfw.WindowHint(glfw.Resizable, glfw.False)
	window, err := glfw.CreateWindow(cfg.Width, cfg.Height, cfg.Title, nil, nil)
	if err != nil {
		panic(fmt.Errorf("gem: glfw.CreateWindow failed: %w", err))
	}

	extensions := window.GetRequiredInstanceExtensions()
	surfaceFunc := func(inst vk.Instance, _ uintptr) (vk.Surface, error) {
		ptr, e := window.CreateWindowSurface(inst, nil)
		if e != nil {
			return vk.NullSurface, e
		}
		return vk.SurfaceFromPointer(ptr), nil
	}
	var vo ash.Vulkan
	if cfg.RayTracing {
		vo, err = ash.NewDeviceWithOptions(cfg.Title, extensions, surfaceFunc, 0, rtDeviceOptions())
	} else {
		vo, err = ash.NewDevice(cfg.Title, extensions, surfaceFunc, 0)
	}
	if err != nil {
		panic(err)
	}

	windowSize := ash.NewExtentSize(cfg.Width, cfg.Height)
	swapchain, err := ash.NewSwapchain(vo.Device, vo.GpuDevice, vo.Surface, windowSize)
	if err != nil {
		panic(err)
	}

	renderer, err := ash.NewRenderer(vo.Device, swapchain.DisplayFormat)
	if err != nil {
		panic(err)
	}
	if err := swapchain.CreateFramebuffers(renderer.RenderPass, vk.NullImageView); err != nil {
		panic(err)
	}
	if err := renderer.CreateCommandBuffers(swapchain.DefaultSwapchainLen()); err != nil {
		panic(err)
	}

	fence, semaphore, err := ash.NewSyncObjects(vo.Device)
	if err != nil {
		panic(err)
	}

	in := input.New()
	hookInput(window, in)

	return &Engine{
		Config:         cfg,
		Input:          in,
		Window:         window,
		vo:             vo,
		swapchain:      swapchain,
		renderer:       renderer,
		fence:          fence,
		semaphore:      semaphore,
		lastTime:       time.Now(),
		vertShaderData: make(map[string]*[]byte),
		fragShaderData: make(map[string]*[]byte),
	}
}

func (e *Engine) setRasterShaders(rs *rasterState, vertShaderData, fragShaderData []byte) error {
	if len(vertShaderData) == 0 {
		return fmt.Errorf("gem: vertex shader data is empty")
	}
	if len(fragShaderData) == 0 {
		return fmt.Errorf("gem: fragment shader data is empty")
	}
	if err := e.ensureRasterBuffer(rs); err != nil {
		return err
	}

	gfx, err := ash.NewGraphicsPipelineWithOptions(e.vo.Device, e.swapchain.DisplaySize, e.renderer.RenderPass, ash.PipelineOptions{
		VertShaderData: vertShaderData,
		FragShaderData: fragShaderData,
		PushConstantRanges: []vk.PushConstantRange{{
			StageFlags: vk.ShaderStageFlags(vk.ShaderStageVertexBit | vk.ShaderStageFragmentBit),
			Offset:     0,
			Size:       pushConstSize,
		}},
	})
	if err != nil {
		return err
	}

	if rs.gfx.GetPipeline() != vk.NullPipeline {
		rs.gfx.Destroy()
	}
	rs.gfx = gfx
	return nil
}

func (e *Engine) LoadShaders(vertShaderPath, fragShaderPath string) error {
	aaa, err := os.ReadFile(vertShaderPath)
	if err != nil {
		return fmt.Errorf("gem: read vertex shader %q: %w", vertShaderPath, err)
	}
	e.vertShaderData[filepath.Base(vertShaderPath)] = &aaa

	bbb, err2 := os.ReadFile(fragShaderPath)
	if err2 != nil {
		return fmt.Errorf("gem: read fragment shader %q: %w", fragShaderPath, err2)
	}
	e.fragShaderData[filepath.Base(fragShaderPath)] = &bbb

	return nil
}

func (e *Engine) GetVertexShader(name string) *[]byte {
	if ptr, ok := e.vertShaderData[name]; ok {
		return ptr
	}
	return nil
}

func (e *Engine) GetFragmentShader(name string) *[]byte {
	if ptr, ok := e.fragShaderData[name]; ok {
		return ptr
	}
	return nil
}

func (e *Engine) ensureRasterBuffer(rs *rasterState) error {
	if rs.buffer.GetDeviceMemory() != vk.NullDeviceMemory {
		return nil
	}

	sz := float32(0.1)
	vertices := []float32{
		0, -sz, 0,
		sz * float32(math.Sin(2*math.Pi/3)), -sz * float32(math.Cos(2*math.Pi/3)), 0,
		sz * float32(math.Sin(4*math.Pi/3)), -sz * float32(math.Cos(4*math.Pi/3)), 0,
	}
	buffer, err := ash.NewBufferWithData(e.vo.Device, e.vo.GpuDevice, vertices)
	if err != nil {
		return err
	}
	rs.buffer = buffer
	return nil
}

// Run starts the game loop with the given scene.
// The loop ends when the window is closed or Scene.Update returns true.
// Call Scene.Init before the first Run if the scene needs initialization.
func (e *Engine) Run(scene Scene) {
	if _, ok := scene.(CustomDrawer); !ok {
		rs, ok := scene.(rasterScene)
		if !ok || rs.rasterState() == nil {
			panic("gem: raster scene must embed SceneBasic")
		}
		if rs.rasterState().gfx.GetPipeline() == vk.NullPipeline {
			panic("gem: scene shaders not set; call scene.LoadShaders or scene.SetShaders before Run")
		}
		e.activeRaster = rs.rasterState()
	}

	e.elapsed = 0
	e.lastTime = time.Now()
	for !e.Window.ShouldClose() {
		now := time.Now()
		e.dt = now.Sub(e.lastTime).Seconds()
		e.elapsed += e.dt
		e.lastTime = now

		e.Input.Tick(e.elapsed)
		glfw.PollEvents()

		if scene.Update(e) {
			break
		}

		if cd, ok := scene.(CustomDrawer); ok {
			if !cd.DrawCustom(e) {
				break
			}
		} else {
			if !e.beginFrame() {
				continue
			}
			scene.Draw(e)
			e.endFrame()
		}
	}
}

func (e *Engine) beginFrame() bool {
	var nextIdx uint32
	ret := vk.AcquireNextImage(e.vo.Device, e.swapchain.DefaultSwapchain(), vk.MaxUint64, e.semaphore, vk.NullFence, &nextIdx)
	if ret != vk.Success && ret != vk.Suboptimal {
		return false
	}
	e.frameIdx = nextIdx
	e.cmd = e.renderer.GetCmdBuffers()[nextIdx]

	vk.ResetCommandBuffer(e.cmd, 0)
	vk.BeginCommandBuffer(e.cmd, &vk.CommandBufferBeginInfo{SType: vk.StructureTypeCommandBufferBeginInfo})

	clearValues := []vk.ClearValue{vk.NewClearValue([]float32{0.05, 0.05, 0.05, 1})}
	vk.CmdBeginRenderPass(e.cmd, &vk.RenderPassBeginInfo{
		SType:           vk.StructureTypeRenderPassBeginInfo,
		RenderPass:      e.renderer.RenderPass,
		Framebuffer:     e.swapchain.Framebuffers[nextIdx],
		RenderArea:      vk.Rect2D{Extent: e.swapchain.DisplaySize},
		ClearValueCount: 1,
		PClearValues:    clearValues,
	}, vk.SubpassContentsInline)

	vk.CmdBindPipeline(e.cmd, vk.PipelineBindPointGraphics, e.activeRaster.gfx.GetPipeline())
	vk.CmdBindVertexBuffers(e.cmd, 0, 1, []vk.Buffer{e.activeRaster.buffer.DefaultVertexBuffer()}, []vk.DeviceSize{0})

	e.inFrame = true
	return true
}

func (e *Engine) endFrame() {
	if !e.inFrame {
		return
	}
	e.inFrame = false

	vk.CmdEndRenderPass(e.cmd)
	vk.EndCommandBuffer(e.cmd)

	vk.ResetFences(e.vo.Device, 1, []vk.Fence{e.fence})
	vk.QueueSubmit(e.vo.Queue, 1, []vk.SubmitInfo{{
		SType:              vk.StructureTypeSubmitInfo,
		WaitSemaphoreCount: 1,
		PWaitSemaphores:    []vk.Semaphore{e.semaphore},
		PWaitDstStageMask:  []vk.PipelineStageFlags{vk.PipelineStageFlags(vk.PipelineStageColorAttachmentOutputBit)},
		CommandBufferCount: 1,
		PCommandBuffers:    e.renderer.GetCmdBuffers()[e.frameIdx:],
	}}, e.fence)
	vk.WaitForFences(e.vo.Device, 1, []vk.Fence{e.fence}, vk.True, 10_000_000_000)

	vk.QueuePresent(e.vo.Queue, &vk.PresentInfo{
		SType:          vk.StructureTypePresentInfo,
		SwapchainCount: 1,
		PSwapchains:    e.swapchain.Swapchains,
		PImageIndices:  []uint32{e.frameIdx},
	})
}

// DrawTriangle draws a colored triangle at position (x, y) with rotation angle (radians).
func (e *Engine) DrawTriangle(position component.Position, angle component.Angle, color component.Color) {
	if !e.inFrame || e.activeRaster == nil {
		return
	}
	pc := pushConstants{
		OffsetX:    position.X,
		OffsetY:    position.Y,
		Angle:      float32(angle),
		Aspect:     float32(e.Config.Height) / float32(e.Config.Width),
		ColorR:     color.ColorR,
		ColorG:     color.ColorG,
		ColorB:     color.ColorB,
		Brightness: 1.0,
	}
	flags := vk.ShaderStageFlags(vk.ShaderStageVertexBit | vk.ShaderStageFragmentBit)
	vk.CmdPushConstants(e.cmd, e.activeRaster.gfx.GetLayout(), flags, 0, pushConstSize, unsafe.Pointer(&pc))
	vk.CmdDraw(e.cmd, 3, 1, 0, 0)
}

// Elapsed returns total time since the engine started, in seconds.
func (e *Engine) Elapsed() float64 { return e.elapsed }

// DeltaTime returns duration of the last frame, in seconds.
func (e *Engine) DeltaTime() float64 { return e.dt }

// Destroy releases all Vulkan and window resources.
func (e *Engine) Destroy() {
	vk.DeviceWaitIdle(e.vo.Device)
	vk.DestroySemaphore(e.vo.Device, e.semaphore, nil)
	vk.DestroyFence(e.vo.Device, e.fence, nil)
	e.destroyRaster(e.activeRaster)
	vk.FreeCommandBuffers(e.vo.Device, e.renderer.GetCmdPool(), uint32(len(e.renderer.GetCmdBuffers())), e.renderer.GetCmdBuffers())
	vk.DestroyCommandPool(e.vo.Device, e.renderer.GetCmdPool(), nil)
	vk.DestroyRenderPass(e.vo.Device, e.renderer.RenderPass, nil)
	e.swapchain.Destroy()
	vk.DestroyDevice(e.vo.Device, nil)
	if e.vo.GetDebugCallback() != vk.NullDebugReportCallback {
		vk.DestroyDebugReportCallback(e.vo.Instance, e.vo.GetDebugCallback(), nil)
	}
	vk.DestroySurface(e.vo.Instance, e.vo.Surface, nil)
	vk.DestroyInstance(e.vo.Instance, nil)
	e.Window.Destroy()
	glfw.Terminate()
}

func (e *Engine) destroyRaster(rs *rasterState) {
	if rs == nil {
		return
	}
	if rs.gfx.GetPipeline() != vk.NullPipeline {
		rs.gfx.Destroy()
		rs.gfx = ash.VulkanGfxPipelineInfo{}
	}
	if rs.buffer.GetDeviceMemory() != vk.NullDeviceMemory {
		vk.FreeMemory(e.vo.Device, rs.buffer.GetDeviceMemory(), nil)
	}
	rs.buffer.Destroy()
	rs.buffer = ash.VulkanBufferInfo{}
	if e.activeRaster == rs {
		e.activeRaster = nil
	}
}

// CleanShadersBank clear map with pointers to shader data
func (e *Engine) CleanShadersBank() {
	clear(e.vertShaderData)
	clear(e.fragShaderData)
}
