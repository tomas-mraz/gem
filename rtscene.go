package gem

import (
	"log"
	"math"
	"unsafe"

	vk "github.com/tomas-mraz/vulkan"
	ash "github.com/tomas-mraz/vulkan-ash"
)

// RTConfig configures a ray-traced scene.
type RTConfig struct {
	ModelPath        string
	RaygenShader     []byte
	MissShader       []byte
	ShadowMissShader []byte
	ClosestHitShader []byte
	AnyHitShader     []byte
	CameraPosition   [3]float32
	CameraFOV        float32
	CameraNear       float32
	CameraFar        float32
}

// RTScene manages all Vulkan ray tracing resources for a scene.
type RTScene struct {
	model          rtModelData
	tlasBuf        vk.Buffer
	tlasMem        vk.DeviceMemory
	tlas           vk.AccelerationStructure
	storageImg     ash.VulkanStorageImageInfo
	uniforms       ash.VulkanUniformBuffers
	descLayout     vk.DescriptorSetLayout
	descPool       vk.DescriptorPool
	descSets       []vk.DescriptorSet
	pipelineLayout vk.PipelineLayout
	pipeline       vk.Pipeline
	raygenSBT      vk.StridedDeviceAddressRegion
	missSBT        vk.StridedDeviceAddressRegion
	hitSBT         vk.StridedDeviceAddressRegion
	sbtBuf         vk.Buffer
	sbtMem         vk.DeviceMemory
	projMatrix     ash.Mat4x4
	viewMatrix     ash.Mat4x4
	frameCounter   uint32
	width          uint32
	height         uint32
}

// rtUniformData matches the shader uniform buffer layout.
type rtUniformData struct {
	ViewInverse ash.Mat4x4
	ProjInverse ash.Mat4x4
	Frame       uint32
	Pad         [3]uint32
	LightPos    [4]float32
}

var rtUniformSize = int(unsafe.Sizeof(rtUniformData{}))

func (u *rtUniformData) bytes() []byte {
	return unsafe.Slice((*byte)(unsafe.Pointer(u)), rtUniformSize)
}

type rtGeometryNode struct {
	VertexBufferDeviceAddress uint64
	IndexBufferDeviceAddress  uint64
	TextureIndexBaseColor     int32
	TextureIndexOcclusion     int32
}

// NewRTScene creates a ray-traced scene from the given configuration.
func NewRTScene(e *Engine, cfg *RTConfig) *RTScene {
	dev := e.vo.Device
	gpu := e.vo.GpuDevice
	queue := e.vo.Queue
	cmdPool := e.renderer.GetCmdPool()
	swapchainLen := e.swapchain.DefaultSwapchainLen()
	width := uint32(e.Config.Width)
	height := uint32(e.Config.Height)

	rt := &RTScene{width: width, height: height}

	rt.model = rtLoadGLTFModel(dev, gpu, queue, cmdPool, cfg.ModelPath)
	log.Printf("Loaded %d primitives into one BLAS", len(rt.model.primitives))

	rt.tlasBuf, rt.tlasMem, rt.tlas = rtBuildTLAS(dev, gpu, queue, cmdPool, rt.model.blas)

	storageImg, err := ash.NewStorageImage(dev, gpu, queue, cmdPool, width, height, e.swapchain.DisplayFormat)
	if err != nil {
		log.Fatal(err)
	}
	rt.storageImg = storageImg

	uniforms, err := ash.NewUniformBuffers(dev, gpu, swapchainLen, rtUniformSize)
	if err != nil {
		log.Fatal(err)
	}
	rt.uniforms = uniforms

	rt.descLayout, rt.descPool, rt.descSets = rtCreateDescriptorSets(dev, swapchainLen, rt.tlas, rt.storageImg.GetView(), rt.model.geometryBuf, rt.model.textures, &rt.uniforms)
	rt.pipelineLayout, rt.pipeline = rtCreatePipeline(dev, rt.descLayout, cfg)

	const shaderGroupHandleSize = 32
	const shaderGroupHandleAlignment = 32
	rt.raygenSBT, rt.missSBT, rt.hitSBT, rt.sbtBuf, rt.sbtMem = rtCreateSBT(dev, gpu, rt.pipeline, shaderGroupHandleSize, shaderGroupHandleAlignment)

	rtSetPerspectiveZO(&rt.projMatrix, ash.DegreesToRadians(cfg.CameraFOV), float32(width)/float32(height), cfg.CameraNear, cfg.CameraFar)
	rt.viewMatrix.Translate(cfg.CameraPosition[0], cfg.CameraPosition[1], cfg.CameraPosition[2])

	log.Println("Ray tracing scene initialized")
	return rt
}

// Render draws one ray-traced frame with the given light position.
func (rt *RTScene) Render(e *Engine, lightPos [4]float32) bool {
	rt.frameCounter = 0 // reset accumulation — light moved

	dev := e.vo.Device
	queue := e.vo.Queue
	swapInfo := e.swapchain
	cmdBuffers := e.renderer.GetCmdBuffers()
	fence := e.fence
	semaphore := e.semaphore

	var nextIdx uint32
	ret := vk.AcquireNextImage(dev, swapInfo.DefaultSwapchain(), vk.MaxUint64, semaphore, vk.NullFence, &nextIdx)
	if ret != vk.Success && ret != vk.Suboptimal {
		return false
	}

	projInv := ash.InvertMatrix(&rt.projMatrix)
	viewInv := ash.InvertMatrix(&rt.viewMatrix)
	ubo := rtUniformData{ViewInverse: viewInv, ProjInverse: projInv, Frame: rt.frameCounter, LightPos: lightPos}
	rt.uniforms.Update(nextIdx, ubo.bytes())
	rt.frameCounter++

	cmd := cmdBuffers[nextIdx]
	vk.ResetCommandBuffer(cmd, 0)
	vk.BeginCommandBuffer(cmd, &vk.CommandBufferBeginInfo{SType: vk.StructureTypeCommandBufferBeginInfo})

	vk.CmdBindPipeline(cmd, vk.PipelineBindPointRayTracing, rt.pipeline)
	vk.CmdBindDescriptorSets(cmd, vk.PipelineBindPointRayTracing, rt.pipelineLayout, 0, 1, []vk.DescriptorSet{rt.descSets[nextIdx]}, 0, nil)

	emptySBT := vk.StridedDeviceAddressRegion{}
	vk.CmdTraceRays(cmd, &rt.raygenSBT, &rt.missSBT, &rt.hitSBT, &emptySBT, rt.width, rt.height, 1)

	// Copy storage image to swapchain image
	subresourceRange := vk.ImageSubresourceRange{AspectMask: vk.ImageAspectFlags(vk.ImageAspectColorBit), LevelCount: 1, LayerCount: 1}

	var imgCount uint32
	vk.GetSwapchainImages(dev, swapInfo.DefaultSwapchain(), &imgCount, nil)
	swapImages := make([]vk.Image, imgCount)
	vk.GetSwapchainImages(dev, swapInfo.DefaultSwapchain(), &imgCount, swapImages)

	// Transition swapchain image to transfer dst
	vk.CmdPipelineBarrier(cmd, vk.PipelineStageFlags(vk.PipelineStageAllCommandsBit), vk.PipelineStageFlags(vk.PipelineStageAllCommandsBit),
		0, 0, nil, 0, nil, 1, []vk.ImageMemoryBarrier{{
			SType: vk.StructureTypeImageMemoryBarrier, OldLayout: vk.ImageLayoutUndefined, NewLayout: vk.ImageLayoutTransferDstOptimal,
			Image: swapImages[nextIdx], SubresourceRange: subresourceRange, DstAccessMask: vk.AccessFlags(vk.AccessTransferWriteBit),
			SrcQueueFamilyIndex: vk.QueueFamilyIgnored, DstQueueFamilyIndex: vk.QueueFamilyIgnored,
		}})
	// Transition storage image to transfer src
	vk.CmdPipelineBarrier(cmd, vk.PipelineStageFlags(vk.PipelineStageAllCommandsBit), vk.PipelineStageFlags(vk.PipelineStageAllCommandsBit),
		0, 0, nil, 0, nil, 1, []vk.ImageMemoryBarrier{{
			SType: vk.StructureTypeImageMemoryBarrier, OldLayout: vk.ImageLayoutGeneral, NewLayout: vk.ImageLayoutTransferSrcOptimal,
			Image: rt.storageImg.GetImage(), SubresourceRange: subresourceRange,
			SrcAccessMask: vk.AccessFlags(vk.AccessShaderWriteBit), DstAccessMask: vk.AccessFlags(vk.AccessTransferReadBit),
			SrcQueueFamilyIndex: vk.QueueFamilyIgnored, DstQueueFamilyIndex: vk.QueueFamilyIgnored,
		}})

	// Copy
	vk.CmdCopyImage(cmd, rt.storageImg.GetImage(), vk.ImageLayoutTransferSrcOptimal, swapImages[nextIdx], vk.ImageLayoutTransferDstOptimal,
		1, []vk.ImageCopy{{
			SrcSubresource: vk.ImageSubresourceLayers{AspectMask: vk.ImageAspectFlags(vk.ImageAspectColorBit), LayerCount: 1},
			DstSubresource: vk.ImageSubresourceLayers{AspectMask: vk.ImageAspectFlags(vk.ImageAspectColorBit), LayerCount: 1},
			Extent:         vk.Extent3D{Width: rt.width, Height: rt.height, Depth: 1},
		}})

	// Transition swapchain image to present
	vk.CmdPipelineBarrier(cmd, vk.PipelineStageFlags(vk.PipelineStageAllCommandsBit), vk.PipelineStageFlags(vk.PipelineStageAllCommandsBit),
		0, 0, nil, 0, nil, 1, []vk.ImageMemoryBarrier{{
			SType: vk.StructureTypeImageMemoryBarrier, OldLayout: vk.ImageLayoutTransferDstOptimal, NewLayout: vk.ImageLayoutPresentSrc,
			Image: swapImages[nextIdx], SubresourceRange: subresourceRange,
			SrcAccessMask:       vk.AccessFlags(vk.AccessTransferWriteBit),
			SrcQueueFamilyIndex: vk.QueueFamilyIgnored, DstQueueFamilyIndex: vk.QueueFamilyIgnored,
		}})
	// Transition storage image back to general
	vk.CmdPipelineBarrier(cmd, vk.PipelineStageFlags(vk.PipelineStageAllCommandsBit), vk.PipelineStageFlags(vk.PipelineStageAllCommandsBit),
		0, 0, nil, 0, nil, 1, []vk.ImageMemoryBarrier{{
			SType: vk.StructureTypeImageMemoryBarrier, OldLayout: vk.ImageLayoutTransferSrcOptimal, NewLayout: vk.ImageLayoutGeneral,
			Image: rt.storageImg.GetImage(), SubresourceRange: subresourceRange,
			SrcAccessMask: vk.AccessFlags(vk.AccessTransferReadBit), DstAccessMask: vk.AccessFlags(vk.AccessShaderWriteBit),
			SrcQueueFamilyIndex: vk.QueueFamilyIgnored, DstQueueFamilyIndex: vk.QueueFamilyIgnored,
		}})

	vk.EndCommandBuffer(cmd)

	vk.ResetFences(dev, 1, []vk.Fence{fence})
	if err := vk.Error(vk.QueueSubmit(queue, 1, []vk.SubmitInfo{{
		SType: vk.StructureTypeSubmitInfo, WaitSemaphoreCount: 1, PWaitSemaphores: []vk.Semaphore{semaphore},
		PWaitDstStageMask:  []vk.PipelineStageFlags{vk.PipelineStageFlags(vk.PipelineStageColorAttachmentOutputBit)},
		CommandBufferCount: 1, PCommandBuffers: cmdBuffers[nextIdx:],
	}}, fence)); err != nil {
		log.Println("QueueSubmit:", err)
		return false
	}
	if err := vk.Error(vk.WaitForFences(dev, 1, []vk.Fence{fence}, vk.True, 10_000_000_000)); err != nil {
		log.Println("WaitForFences:", err)
		return false
	}
	ret = vk.QueuePresent(queue, &vk.PresentInfo{
		SType: vk.StructureTypePresentInfo, SwapchainCount: 1, PSwapchains: swapInfo.Swapchains, PImageIndices: []uint32{nextIdx},
	})
	return ret == vk.Success || ret == vk.Suboptimal
}

// Destroy releases all ray tracing resources.
func (rt *RTScene) Destroy(dev vk.Device) {
	vk.DeviceWaitIdle(dev)
	vk.DestroyBuffer(dev, rt.sbtBuf, nil)
	vk.FreeMemory(dev, rt.sbtMem, nil)
	vk.DestroyPipeline(dev, rt.pipeline, nil)
	vk.DestroyPipelineLayout(dev, rt.pipelineLayout, nil)
	vk.DestroyDescriptorPool(dev, rt.descPool, nil)
	vk.DestroyDescriptorSetLayout(dev, rt.descLayout, nil)
	rt.uniforms.Destroy()
	rt.storageImg.Destroy()
	for i := range rt.model.textures {
		rtDestroyTexture(dev, rt.model.textures[i])
	}
	vk.DestroyAccelerationStructure(dev, rt.tlas, nil)
	vk.DestroyBuffer(dev, rt.tlasBuf, nil)
	vk.FreeMemory(dev, rt.tlasMem, nil)
	vk.DestroyAccelerationStructure(dev, rt.model.blas, nil)
	vk.DestroyBuffer(dev, rt.model.blasBuf, nil)
	vk.FreeMemory(dev, rt.model.blasMem, nil)
	vk.DestroyBuffer(dev, rt.model.geometryBuf, nil)
	vk.FreeMemory(dev, rt.model.geometryMem, nil)
	for i := range rt.model.primitives {
		vk.DestroyBuffer(dev, rt.model.primitives[i].indexBuf, nil)
		vk.FreeMemory(dev, rt.model.primitives[i].indexMem, nil)
		vk.DestroyBuffer(dev, rt.model.primitives[i].vertexBuf, nil)
		vk.FreeMemory(dev, rt.model.primitives[i].vertexMem, nil)
	}
}

// DestroyWithEngine releases all resources using the engine's device.
func (rt *RTScene) DestroyWithEngine(e *Engine) {
	rt.Destroy(e.vo.Device)
}

// --- RT device setup ---

func rtDeviceOptions() *ash.DeviceOptions {
	// These structs live on the caller's stack (engine.New) — safe because
	// NewDeviceWithOptions consumes them synchronously.
	bufDevAddr := vk.PhysicalDeviceBufferDeviceAddressFeatures{
		SType:               vk.StructureTypePhysicalDeviceBufferDeviceAddressFeatures,
		BufferDeviceAddress: vk.True,
	}
	rtPipeline := vk.PhysicalDeviceRayTracingPipelineFeatures{
		SType:              vk.StructureTypePhysicalDeviceRayTracingPipelineFeatures,
		RayTracingPipeline: vk.True,
		PNext:              unsafe.Pointer(&bufDevAddr),
	}
	asFeat := vk.PhysicalDeviceAccelerationStructureFeatures{
		SType:                 vk.StructureTypePhysicalDeviceAccelerationStructureFeatures,
		AccelerationStructure: vk.True,
		PNext:                 unsafe.Pointer(&rtPipeline),
	}
	descIdx := vk.PhysicalDeviceDescriptorIndexingFeatures{
		SType:                                    vk.StructureTypePhysicalDeviceDescriptorIndexingFeatures,
		ShaderSampledImageArrayNonUniformIndexing: vk.True,
		DescriptorBindingVariableDescriptorCount:  vk.True,
		RuntimeDescriptorArray:                    vk.True,
		PNext:                                     unsafe.Pointer(&asFeat),
	}
	enabledFeatures := vk.PhysicalDeviceFeatures{
		ShaderInt64:                          vk.True,
		ShaderStorageImageReadWithoutFormat:  vk.True,
		ShaderStorageImageWriteWithoutFormat: vk.True,
	}
	return &ash.DeviceOptions{
		DeviceExtensions: []string{
			"VK_KHR_acceleration_structure\x00",
			"VK_KHR_ray_tracing_pipeline\x00",
			"VK_KHR_buffer_device_address\x00",
			"VK_KHR_deferred_host_operations\x00",
			"VK_EXT_descriptor_indexing\x00",
			"VK_KHR_spirv_1_4\x00",
			"VK_KHR_shader_float_controls\x00",
		},
		PNextChain:      unsafe.Pointer(&descIdx),
		EnabledFeatures: &enabledFeatures,
		ApiVersion:      vk.MakeVersion(1, 2, 0),
	}
}

// --- RT helpers ---

func rtBeginOneTimeCmd(dev vk.Device, cmdPool vk.CommandPool) vk.CommandBuffer {
	cmds := make([]vk.CommandBuffer, 1)
	vk.AllocateCommandBuffers(dev, &vk.CommandBufferAllocateInfo{
		SType:              vk.StructureTypeCommandBufferAllocateInfo,
		CommandPool:        cmdPool,
		Level:              vk.CommandBufferLevelPrimary,
		CommandBufferCount: 1,
	}, cmds)
	vk.BeginCommandBuffer(cmds[0], &vk.CommandBufferBeginInfo{
		SType: vk.StructureTypeCommandBufferBeginInfo,
		Flags: vk.CommandBufferUsageFlags(vk.CommandBufferUsageOneTimeSubmitBit),
	})
	return cmds[0]
}

func rtEndOneTimeCmd(dev vk.Device, queue vk.Queue, cmdPool vk.CommandPool, cmd vk.CommandBuffer) {
	vk.EndCommandBuffer(cmd)
	var fence vk.Fence
	vk.CreateFence(dev, &vk.FenceCreateInfo{SType: vk.StructureTypeFenceCreateInfo}, nil, &fence)
	vk.QueueSubmit(queue, 1, []vk.SubmitInfo{{
		SType: vk.StructureTypeSubmitInfo, CommandBufferCount: 1,
		PCommandBuffers: []vk.CommandBuffer{cmd},
	}}, fence)
	vk.WaitForFences(dev, 1, []vk.Fence{fence}, vk.True, 10_000_000_000)
	vk.DestroyFence(dev, fence, nil)
	vk.FreeCommandBuffers(dev, cmdPool, 1, []vk.CommandBuffer{cmd})
}

func rtSetDeviceAddressConst(addr *vk.DeviceOrHostAddressConst, da vk.DeviceAddress) {
	*(*vk.DeviceAddress)(unsafe.Pointer(&addr[0])) = da
}

func rtSetDeviceAddress(addr *vk.DeviceOrHostAddress, da vk.DeviceAddress) {
	*(*vk.DeviceAddress)(unsafe.Pointer(&addr[0])) = da
}

func rtSetGeometryTriangles(data *vk.AccelerationStructureGeometryData, tri *vk.AccelerationStructureGeometryTrianglesData) {
	cTri, _ := tri.PassRef()
	src := unsafe.Slice((*byte)(unsafe.Pointer(cTri)), len(*data))
	copy((*data)[:], src)
}

func rtSetGeometryInstances(data *vk.AccelerationStructureGeometryData, inst *vk.AccelerationStructureGeometryInstancesData) {
	cInst, _ := inst.PassRef()
	src := unsafe.Slice((*byte)(unsafe.Pointer(cInst)), len(*data))
	copy((*data)[:], src)
}

func rtSetPerspectiveZO(m *ash.Mat4x4, yFov, aspect, near, far float32) {
	f := float32(1.0 / math.Tan(float64(yFov)/2.0))
	m[0][0] = f / aspect
	m[0][1] = 0
	m[0][2] = 0
	m[0][3] = 0
	m[1][0] = 0
	m[1][1] = f
	m[1][2] = 0
	m[1][3] = 0
	m[2][0] = 0
	m[2][1] = 0
	m[2][2] = far / (near - far)
	m[2][3] = -1
	m[3][0] = 0
	m[3][1] = 0
	m[3][2] = (near * far) / (near - far)
	m[3][3] = 0
}

// --- Acceleration structures ---

func rtBuildBLAS(dev vk.Device, gpu vk.PhysicalDevice, queue vk.Queue, cmdPool vk.CommandPool, prims []rtPrimitiveData) (vk.Buffer, vk.DeviceMemory, vk.AccelerationStructure) {
	geometries := make([]vk.AccelerationStructureGeometry, 0, len(prims))
	primitiveCounts := make([]uint32, 0, len(prims))
	rangeInfos := make([]vk.AccelerationStructureBuildRangeInfo, 0, len(prims))
	transformMatrices := make([][12]float32, len(prims))

	for i := range prims {
		transformMatrices[i] = prims[i].transform
	}
	transformBuf, transformMem, err := ash.NewBufferWithDeviceAddress(dev, gpu,
		vk.BufferUsageFlags(vk.BufferUsageShaderDeviceAddressBit|vk.BufferUsageAccelerationStructureBuildInputReadOnlyBit),
		uint64(len(transformMatrices))*uint64(unsafe.Sizeof(transformMatrices[0])),
		unsafe.Pointer(&transformMatrices[0]))
	if err != nil {
		log.Fatal(err)
	}
	transformAddr := ash.GetBufferDeviceAddress(dev, transformBuf)
	transformStride := vk.DeviceAddress(unsafe.Sizeof(transformMatrices[0]))

	for i := range prims {
		vertexAddr := ash.GetBufferDeviceAddress(dev, prims[i].vertexBuf)
		indexAddr := ash.GetBufferDeviceAddress(dev, prims[i].indexBuf)

		var trianglesData vk.AccelerationStructureGeometryTrianglesData
		trianglesData.SType = vk.StructureTypeAccelerationStructureGeometryTrianglesData
		trianglesData.VertexFormat = vk.FormatR32g32b32Sfloat
		rtSetDeviceAddressConst(&trianglesData.VertexData, vertexAddr)
		trianglesData.VertexStride = 32
		trianglesData.MaxVertex = prims[i].vertexCount - 1
		trianglesData.IndexType = vk.IndexTypeUint32
		rtSetDeviceAddressConst(&trianglesData.IndexData, indexAddr)
		rtSetDeviceAddressConst(&trianglesData.TransformData, transformAddr+vk.DeviceAddress(i)*transformStride)

		var geometry vk.AccelerationStructureGeometry
		geometry.SType = vk.StructureTypeAccelerationStructureGeometry
		geometry.GeometryType = vk.GeometryTypeTriangles
		geometry.Flags = vk.GeometryFlags(vk.GeometryOpaqueBit)
		rtSetGeometryTriangles(&geometry.Geometry, &trianglesData)

		geometries = append(geometries, geometry)
		primitiveCounts = append(primitiveCounts, prims[i].triangleCount)
		rangeInfos = append(rangeInfos, vk.AccelerationStructureBuildRangeInfo{
			PrimitiveCount: prims[i].triangleCount,
		})
	}

	buildInfo := vk.AccelerationStructureBuildGeometryInfo{
		SType:         vk.StructureTypeAccelerationStructureBuildGeometryInfo,
		Type:          vk.AccelerationStructureTypeBottomLevel,
		Flags:         vk.BuildAccelerationStructureFlags(vk.BuildAccelerationStructurePreferFastTraceBit),
		GeometryCount: uint32(len(geometries)),
		PGeometries:   geometries,
	}

	var sizeInfo vk.AccelerationStructureBuildSizesInfo
	sizeInfo.SType = vk.StructureTypeAccelerationStructureBuildSizesInfo
	vk.GetAccelerationStructureBuildSizes(dev, vk.AccelerationStructureBuildTypeDevice, &buildInfo, &primitiveCounts[0], &sizeInfo)
	sizeInfo.Deref()
	log.Printf("BLAS size: AS=%d, scratch=%d (geometries=%d)", sizeInfo.AccelerationStructureSize, sizeInfo.BuildScratchSize, len(geometries))

	asBuf, asMem, err := ash.NewDeviceLocalBuffer(dev, gpu,
		vk.BufferUsageFlags(vk.BufferUsageAccelerationStructureStorageBit|vk.BufferUsageShaderDeviceAddressBit),
		uint64(sizeInfo.AccelerationStructureSize))
	if err != nil {
		log.Fatal(err)
	}

	var as vk.AccelerationStructure
	if err := vk.Error(vk.CreateAccelerationStructure(dev, &vk.AccelerationStructureCreateInfo{
		SType: vk.StructureTypeAccelerationStructureCreateInfo, Buffer: asBuf,
		Size: sizeInfo.AccelerationStructureSize, Type: vk.AccelerationStructureTypeBottomLevel,
	}, nil, &as)); err != nil {
		log.Fatal("CreateAccelerationStructure (BLAS):", err)
	}

	scratchBuf, scratchMem, err := ash.NewDeviceLocalBuffer(dev, gpu,
		vk.BufferUsageFlags(vk.BufferUsageStorageBufferBit|vk.BufferUsageShaderDeviceAddressBit),
		uint64(sizeInfo.BuildScratchSize))
	if err != nil {
		log.Fatal(err)
	}
	scratchAddr := ash.GetBufferDeviceAddress(dev, scratchBuf)

	buildInfo2 := vk.AccelerationStructureBuildGeometryInfo{
		SType:                    vk.StructureTypeAccelerationStructureBuildGeometryInfo,
		Type:                     vk.AccelerationStructureTypeBottomLevel,
		Flags:                    vk.BuildAccelerationStructureFlags(vk.BuildAccelerationStructurePreferFastTraceBit),
		Mode:                     vk.BuildAccelerationStructureModeBuild,
		DstAccelerationStructure: as,
		GeometryCount:            uint32(len(geometries)),
		PGeometries:              geometries,
	}
	rtSetDeviceAddress(&buildInfo2.ScratchData, scratchAddr)

	cmd := rtBeginOneTimeCmd(dev, cmdPool)
	vk.CmdBuildAccelerationStructures(cmd, 1, &buildInfo2, [][]vk.AccelerationStructureBuildRangeInfo{rangeInfos})
	rtEndOneTimeCmd(dev, queue, cmdPool, cmd)

	vk.DestroyBuffer(dev, transformBuf, nil)
	vk.FreeMemory(dev, transformMem, nil)
	vk.DestroyBuffer(dev, scratchBuf, nil)
	vk.FreeMemory(dev, scratchMem, nil)
	return asBuf, asMem, as
}

func rtBuildTLAS(dev vk.Device, gpu vk.PhysicalDevice, queue vk.Queue, cmdPool vk.CommandPool, blas vk.AccelerationStructure) (vk.Buffer, vk.DeviceMemory, vk.AccelerationStructure) {
	instanceData := make([]byte, 64)
	transform := [12]float32{1, 0, 0, 0, 0, -1, 0, 0, 0, 0, 1, 0}
	blasAddr := vk.GetAccelerationStructureDeviceAddress(dev, &vk.AccelerationStructureDeviceAddressInfo{
		SType: vk.StructureTypeAccelerationStructureDeviceAddressInfo, AccelerationStructure: blas,
	})

	copy(instanceData[:48], unsafe.Slice((*byte)(unsafe.Pointer(&transform[0])), 48))
	instanceData[48] = 0
	instanceData[49] = 0
	instanceData[50] = 0
	instanceData[51] = 0xFF
	instanceData[52] = 0
	instanceData[53] = 0
	instanceData[54] = 0
	instanceData[55] = 0x01
	*(*uint64)(unsafe.Pointer(&instanceData[56])) = uint64(blasAddr)

	instanceBuf, instanceMem, err := ash.NewBufferWithDeviceAddress(dev, gpu,
		vk.BufferUsageFlags(vk.BufferUsageShaderDeviceAddressBit|vk.BufferUsageAccelerationStructureBuildInputReadOnlyBit),
		uint64(len(instanceData)), unsafe.Pointer(&instanceData[0]))
	if err != nil {
		log.Fatal(err)
	}
	instanceAddr := ash.GetBufferDeviceAddress(dev, instanceBuf)

	var instancesData vk.AccelerationStructureGeometryInstancesData
	instancesData.SType = vk.StructureTypeAccelerationStructureGeometryInstancesData
	rtSetDeviceAddressConst(&instancesData.Data, instanceAddr)

	var geometry vk.AccelerationStructureGeometry
	geometry.SType = vk.StructureTypeAccelerationStructureGeometry
	geometry.GeometryType = vk.GeometryTypeInstances
	geometry.Flags = vk.GeometryFlags(vk.GeometryOpaqueBit)
	rtSetGeometryInstances(&geometry.Geometry, &instancesData)

	buildInfo := vk.AccelerationStructureBuildGeometryInfo{
		SType:         vk.StructureTypeAccelerationStructureBuildGeometryInfo,
		Type:          vk.AccelerationStructureTypeTopLevel,
		Flags:         vk.BuildAccelerationStructureFlags(vk.BuildAccelerationStructurePreferFastTraceBit),
		GeometryCount: 1,
		PGeometries:   []vk.AccelerationStructureGeometry{geometry},
	}

	primitiveCount := uint32(1)
	var sizeInfo vk.AccelerationStructureBuildSizesInfo
	sizeInfo.SType = vk.StructureTypeAccelerationStructureBuildSizesInfo
	vk.GetAccelerationStructureBuildSizes(dev, vk.AccelerationStructureBuildTypeDevice, &buildInfo, &primitiveCount, &sizeInfo)
	sizeInfo.Deref()
	log.Printf("TLAS size: AS=%d, scratch=%d (instances=1)", sizeInfo.AccelerationStructureSize, sizeInfo.BuildScratchSize)

	asBuf, asMem, err := ash.NewDeviceLocalBuffer(dev, gpu,
		vk.BufferUsageFlags(vk.BufferUsageAccelerationStructureStorageBit|vk.BufferUsageShaderDeviceAddressBit),
		uint64(sizeInfo.AccelerationStructureSize))
	if err != nil {
		log.Fatal(err)
	}

	var as vk.AccelerationStructure
	if err := vk.Error(vk.CreateAccelerationStructure(dev, &vk.AccelerationStructureCreateInfo{
		SType: vk.StructureTypeAccelerationStructureCreateInfo, Buffer: asBuf,
		Size: sizeInfo.AccelerationStructureSize, Type: vk.AccelerationStructureTypeTopLevel,
	}, nil, &as)); err != nil {
		log.Fatal("CreateAccelerationStructure (TLAS):", err)
	}

	scratchBuf, scratchMem, err := ash.NewDeviceLocalBuffer(dev, gpu,
		vk.BufferUsageFlags(vk.BufferUsageStorageBufferBit|vk.BufferUsageShaderDeviceAddressBit),
		uint64(sizeInfo.BuildScratchSize))
	if err != nil {
		log.Fatal(err)
	}
	scratchAddr := ash.GetBufferDeviceAddress(dev, scratchBuf)

	buildInfo2 := vk.AccelerationStructureBuildGeometryInfo{
		SType:                    vk.StructureTypeAccelerationStructureBuildGeometryInfo,
		Type:                     vk.AccelerationStructureTypeTopLevel,
		Flags:                    vk.BuildAccelerationStructureFlags(vk.BuildAccelerationStructurePreferFastTraceBit),
		Mode:                     vk.BuildAccelerationStructureModeBuild,
		DstAccelerationStructure: as,
		GeometryCount:            1,
		PGeometries:              []vk.AccelerationStructureGeometry{geometry},
	}
	rtSetDeviceAddress(&buildInfo2.ScratchData, scratchAddr)

	rangeInfos := []vk.AccelerationStructureBuildRangeInfo{{PrimitiveCount: primitiveCount}}

	cmd := rtBeginOneTimeCmd(dev, cmdPool)
	vk.CmdBuildAccelerationStructures(cmd, 1, &buildInfo2, [][]vk.AccelerationStructureBuildRangeInfo{rangeInfos})
	rtEndOneTimeCmd(dev, queue, cmdPool, cmd)

	vk.DestroyBuffer(dev, scratchBuf, nil)
	vk.FreeMemory(dev, scratchMem, nil)
	vk.DestroyBuffer(dev, instanceBuf, nil)
	vk.FreeMemory(dev, instanceMem, nil)
	return asBuf, asMem, as
}

// --- Geometry buffer ---

func rtCreateGeometryNodesBuffer(dev vk.Device, gpu vk.PhysicalDevice, prims []rtPrimitiveData) (vk.Buffer, vk.DeviceMemory) {
	nodes := make([]rtGeometryNode, len(prims))
	for i := range prims {
		nodes[i] = rtGeometryNode{
			VertexBufferDeviceAddress: uint64(ash.GetBufferDeviceAddress(dev, prims[i].vertexBuf)),
			IndexBufferDeviceAddress:  uint64(ash.GetBufferDeviceAddress(dev, prims[i].indexBuf)),
			TextureIndexBaseColor:     prims[i].baseColorTex,
			TextureIndexOcclusion:     prims[i].occlusionTex,
		}
	}
	buf, mem, err := ash.NewBufferWithDeviceAddress(dev, gpu,
		vk.BufferUsageFlags(vk.BufferUsageStorageBufferBit|vk.BufferUsageShaderDeviceAddressBit),
		uint64(len(nodes))*uint64(unsafe.Sizeof(nodes[0])),
		unsafe.Pointer(&nodes[0]))
	if err != nil {
		log.Fatal(err)
	}
	return buf, mem
}

// --- Descriptors ---

func rtCreateDescriptorSets(dev vk.Device, count uint32, tlas vk.AccelerationStructure, storageImageView vk.ImageView, geometryBuf vk.Buffer, textures []rtTextureData, uniforms *ash.VulkanUniformBuffers) (vk.DescriptorSetLayout, vk.DescriptorPool, []vk.DescriptorSet) {
	textureCount := uint32(len(textures))
	if textureCount == 0 {
		log.Fatal("rtCreateDescriptorSets: texture array must contain at least the fallback texture")
	}

	immutableFallbackSampler := []vk.Sampler{textures[0].sampler}
	immutableTextureSamplers := make([]vk.Sampler, 0, len(textures))
	for _, texture := range textures {
		immutableTextureSamplers = append(immutableTextureSamplers, texture.sampler)
	}

	var layout vk.DescriptorSetLayout
	vk.CreateDescriptorSetLayout(dev, &vk.DescriptorSetLayoutCreateInfo{
		SType: vk.StructureTypeDescriptorSetLayoutCreateInfo, BindingCount: 6,
		PBindings: []vk.DescriptorSetLayoutBinding{
			{Binding: 0, DescriptorType: vk.DescriptorTypeAccelerationStructure, DescriptorCount: 1, StageFlags: vk.ShaderStageFlags(vk.ShaderStageRaygenBit | vk.ShaderStageClosestHitBit)},
			{Binding: 1, DescriptorType: vk.DescriptorTypeStorageImage, DescriptorCount: 1, StageFlags: vk.ShaderStageFlags(vk.ShaderStageRaygenBit)},
			{Binding: 2, DescriptorType: vk.DescriptorTypeUniformBuffer, DescriptorCount: 1, StageFlags: vk.ShaderStageFlags(vk.ShaderStageRaygenBit | vk.ShaderStageClosestHitBit | vk.ShaderStageMissBit)},
			{Binding: 3, DescriptorType: vk.DescriptorTypeCombinedImageSampler, DescriptorCount: 1, StageFlags: vk.ShaderStageFlags(vk.ShaderStageClosestHitBit | vk.ShaderStageAnyHitBit), PImmutableSamplers: immutableFallbackSampler},
			{Binding: 4, DescriptorType: vk.DescriptorTypeStorageBuffer, DescriptorCount: 1, StageFlags: vk.ShaderStageFlags(vk.ShaderStageClosestHitBit | vk.ShaderStageAnyHitBit)},
			{Binding: 5, DescriptorType: vk.DescriptorTypeCombinedImageSampler, DescriptorCount: textureCount, StageFlags: vk.ShaderStageFlags(vk.ShaderStageClosestHitBit | vk.ShaderStageAnyHitBit), PImmutableSamplers: immutableTextureSamplers},
		},
	}, nil, &layout)

	var pool vk.DescriptorPool
	vk.CreateDescriptorPool(dev, &vk.DescriptorPoolCreateInfo{
		SType: vk.StructureTypeDescriptorPoolCreateInfo, MaxSets: count, PoolSizeCount: 6,
		PPoolSizes: []vk.DescriptorPoolSize{
			{Type: vk.DescriptorTypeAccelerationStructure, DescriptorCount: count},
			{Type: vk.DescriptorTypeStorageImage, DescriptorCount: count},
			{Type: vk.DescriptorTypeUniformBuffer, DescriptorCount: count},
			{Type: vk.DescriptorTypeCombinedImageSampler, DescriptorCount: count},
			{Type: vk.DescriptorTypeStorageBuffer, DescriptorCount: count},
			{Type: vk.DescriptorTypeCombinedImageSampler, DescriptorCount: count * textureCount},
		},
	}, nil, &pool)

	sets := make([]vk.DescriptorSet, count)
	for i := uint32(0); i < count; i++ {
		vk.AllocateDescriptorSets(dev, &vk.DescriptorSetAllocateInfo{
			SType: vk.StructureTypeDescriptorSetAllocateInfo, DescriptorPool: pool,
			DescriptorSetCount: 1, PSetLayouts: []vk.DescriptorSetLayout{layout},
		}, &sets[i])
	}

	textureInfos := make([]vk.DescriptorImageInfo, 0, len(textures))
	for _, texture := range textures {
		textureInfos = append(textureInfos, vk.DescriptorImageInfo{
			Sampler:     texture.sampler,
			ImageView:   texture.view,
			ImageLayout: vk.ImageLayoutShaderReadOnlyOptimal,
		})
	}

	for i := uint32(0); i < count; i++ {
		asWriteInfo := vk.WriteDescriptorSetAccelerationStructure{
			SType:                      vk.StructureTypeWriteDescriptorSetAccelerationStructure,
			AccelerationStructureCount: 1, PAccelerationStructures: []vk.AccelerationStructure{tlas},
		}
		fallbackTextureInfo := textureInfos[0]
		geometryInfo := vk.DescriptorBufferInfo{Buffer: geometryBuf, Offset: 0, Range: vk.DeviceSize(vk.WholeSize)}
		vk.UpdateDescriptorSets(dev, 6, []vk.WriteDescriptorSet{
			{SType: vk.StructureTypeWriteDescriptorSet, DstSet: sets[i], DstBinding: 0, DescriptorCount: 1,
				DescriptorType: vk.DescriptorTypeAccelerationStructure, PNext: unsafe.Pointer(&asWriteInfo)},
			{SType: vk.StructureTypeWriteDescriptorSet, DstSet: sets[i], DstBinding: 1, DescriptorCount: 1,
				DescriptorType: vk.DescriptorTypeStorageImage,
				PImageInfo:     []vk.DescriptorImageInfo{{ImageView: storageImageView, ImageLayout: vk.ImageLayoutGeneral}}},
			{SType: vk.StructureTypeWriteDescriptorSet, DstSet: sets[i], DstBinding: 2, DescriptorCount: 1,
				DescriptorType: vk.DescriptorTypeUniformBuffer,
				PBufferInfo:    []vk.DescriptorBufferInfo{{Buffer: uniforms.GetBuffer(i), Offset: 0, Range: vk.DeviceSize(rtUniformSize)}}},
			{SType: vk.StructureTypeWriteDescriptorSet, DstSet: sets[i], DstBinding: 3, DescriptorCount: 1,
				DescriptorType: vk.DescriptorTypeCombinedImageSampler,
				PImageInfo:     []vk.DescriptorImageInfo{fallbackTextureInfo}},
			{SType: vk.StructureTypeWriteDescriptorSet, DstSet: sets[i], DstBinding: 4, DescriptorCount: 1,
				DescriptorType: vk.DescriptorTypeStorageBuffer,
				PBufferInfo:    []vk.DescriptorBufferInfo{geometryInfo}},
			{SType: vk.StructureTypeWriteDescriptorSet, DstSet: sets[i], DstBinding: 5, DescriptorCount: textureCount,
				DescriptorType: vk.DescriptorTypeCombinedImageSampler,
				PImageInfo:     textureInfos},
		}, 0, nil)
	}
	return layout, pool, sets
}

// --- RT Pipeline ---

func rtCreatePipeline(dev vk.Device, descLayout vk.DescriptorSetLayout, cfg *RTConfig) (vk.PipelineLayout, vk.Pipeline) {
	var pipelineLayout vk.PipelineLayout
	vk.CreatePipelineLayout(dev, &vk.PipelineLayoutCreateInfo{
		SType: vk.StructureTypePipelineLayoutCreateInfo, SetLayoutCount: 1,
		PSetLayouts: []vk.DescriptorSetLayout{descLayout},
	}, nil, &pipelineLayout)

	raygenModule, _ := ash.LoadShaderFromBytes(dev, cfg.RaygenShader)
	missModule, _ := ash.LoadShaderFromBytes(dev, cfg.MissShader)
	shadowMissModule, _ := ash.LoadShaderFromBytes(dev, cfg.ShadowMissShader)
	closestHitModule, _ := ash.LoadShaderFromBytes(dev, cfg.ClosestHitShader)
	anyHitModule, _ := ash.LoadShaderFromBytes(dev, cfg.AnyHitShader)

	stages := []vk.PipelineShaderStageCreateInfo{
		{SType: vk.StructureTypePipelineShaderStageCreateInfo, Stage: vk.ShaderStageFlagBits(vk.ShaderStageRaygenBit), Module: raygenModule, PName: []byte("main\x00")},
		{SType: vk.StructureTypePipelineShaderStageCreateInfo, Stage: vk.ShaderStageFlagBits(vk.ShaderStageMissBit), Module: missModule, PName: []byte("main\x00")},
		{SType: vk.StructureTypePipelineShaderStageCreateInfo, Stage: vk.ShaderStageFlagBits(vk.ShaderStageMissBit), Module: shadowMissModule, PName: []byte("main\x00")},
		{SType: vk.StructureTypePipelineShaderStageCreateInfo, Stage: vk.ShaderStageFlagBits(vk.ShaderStageClosestHitBit), Module: closestHitModule, PName: []byte("main\x00")},
		{SType: vk.StructureTypePipelineShaderStageCreateInfo, Stage: vk.ShaderStageFlagBits(vk.ShaderStageAnyHitBit), Module: anyHitModule, PName: []byte("main\x00")},
	}
	groups := []vk.RayTracingShaderGroupCreateInfo{
		{SType: vk.StructureTypeRayTracingShaderGroupCreateInfo, Type: vk.RayTracingShaderGroupTypeGeneral,
			GeneralShader: 0, ClosestHitShader: vk.ShaderUnused, AnyHitShader: vk.ShaderUnused, IntersectionShader: vk.ShaderUnused},
		{SType: vk.StructureTypeRayTracingShaderGroupCreateInfo, Type: vk.RayTracingShaderGroupTypeGeneral,
			GeneralShader: 1, ClosestHitShader: vk.ShaderUnused, AnyHitShader: vk.ShaderUnused, IntersectionShader: vk.ShaderUnused},
		{SType: vk.StructureTypeRayTracingShaderGroupCreateInfo, Type: vk.RayTracingShaderGroupTypeGeneral,
			GeneralShader: 2, ClosestHitShader: vk.ShaderUnused, AnyHitShader: vk.ShaderUnused, IntersectionShader: vk.ShaderUnused},
		{SType: vk.StructureTypeRayTracingShaderGroupCreateInfo, Type: vk.RayTracingShaderGroupTypeTrianglesHitGroup,
			GeneralShader: vk.ShaderUnused, ClosestHitShader: 3, AnyHitShader: 4, IntersectionShader: vk.ShaderUnused},
	}

	createInfo := vk.RayTracingPipelineCreateInfo{
		SType:      vk.StructureTypeRayTracingPipelineCreateInfo,
		StageCount: uint32(len(stages)), PStages: stages,
		GroupCount: uint32(len(groups)), PGroups: groups,
		MaxPipelineRayRecursionDepth: 1, Layout: pipelineLayout,
	}
	var pip vk.Pipeline
	if err := vk.Error(vk.CreateRayTracingPipelines(dev, vk.DeferredOperation(vk.NullHandle), vk.NullPipelineCache, 1, &createInfo, nil, &pip)); err != nil {
		log.Fatal("CreateRayTracingPipelines:", err)
	}

	vk.DestroyShaderModule(dev, raygenModule, nil)
	vk.DestroyShaderModule(dev, missModule, nil)
	vk.DestroyShaderModule(dev, shadowMissModule, nil)
	vk.DestroyShaderModule(dev, closestHitModule, nil)
	vk.DestroyShaderModule(dev, anyHitModule, nil)
	return pipelineLayout, pip
}

// --- Shader Binding Table ---

func rtCreateSBT(dev vk.Device, gpu vk.PhysicalDevice, pipeline vk.Pipeline, handleSize, handleAlignment uint32) (vk.StridedDeviceAddressRegion, vk.StridedDeviceAddressRegion, vk.StridedDeviceAddressRegion, vk.Buffer, vk.DeviceMemory) {
	groupCount := uint32(4)
	handleSizeAligned := ash.AlignUp(handleSize, handleAlignment)
	sbtSize := groupCount * handleSizeAligned
	handleStorage := make([]byte, sbtSize)
	if err := vk.Error(vk.GetRayTracingShaderGroupHandles(dev, pipeline, 0, groupCount, uint64(sbtSize), unsafe.Pointer(&handleStorage[0]))); err != nil {
		log.Fatal("GetRayTracingShaderGroupHandles:", err)
	}

	sbtBuf, sbtMem, err := ash.NewBufferWithDeviceAddress(dev, gpu,
		vk.BufferUsageFlags(vk.BufferUsageShaderBindingTableBit|vk.BufferUsageShaderDeviceAddressBit),
		uint64(sbtSize), unsafe.Pointer(&handleStorage[0]))
	if err != nil {
		log.Fatal(err)
	}
	sbtAddr := ash.GetBufferDeviceAddress(dev, sbtBuf)

	raygenSBT := vk.StridedDeviceAddressRegion{DeviceAddress: sbtAddr, Stride: vk.DeviceSize(handleSizeAligned), Size: vk.DeviceSize(handleSizeAligned)}
	missSBT := vk.StridedDeviceAddressRegion{DeviceAddress: sbtAddr + vk.DeviceAddress(handleSizeAligned), Stride: vk.DeviceSize(handleSizeAligned), Size: vk.DeviceSize(2 * handleSizeAligned)}
	hitSBT := vk.StridedDeviceAddressRegion{DeviceAddress: sbtAddr + vk.DeviceAddress(3*handleSizeAligned), Stride: vk.DeviceSize(handleSizeAligned), Size: vk.DeviceSize(handleSizeAligned)}
	return raygenSBT, missSBT, hitSBT, sbtBuf, sbtMem
}
