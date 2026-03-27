package gem

import (
	"bytes"
	"fmt"
	"image"
	"image/draw"
	_ "image/jpeg"
	_ "image/png"
	"log"
	"math"
	"os"
	"path/filepath"
	"unsafe"

	"github.com/qmuntal/gltf"
	"github.com/qmuntal/gltf/modeler"
	vk "github.com/tomas-mraz/vulkan"
	ash "github.com/tomas-mraz/vulkan-ash"
)

// rtPrimitiveData holds per-primitive geometry resources.
type rtPrimitiveData struct {
	vertexBuf     vk.Buffer
	vertexMem     vk.DeviceMemory
	indexBuf      vk.Buffer
	indexMem      vk.DeviceMemory
	vertexCount   uint32
	triangleCount uint32
	transform     [12]float32
	baseColorTex  int32
	occlusionTex  int32
}

type rtTextureData struct {
	image   vk.Image
	memory  vk.DeviceMemory
	view    vk.ImageView
	sampler vk.Sampler
}

type rtModelData struct {
	primitives  []rtPrimitiveData
	geometryBuf vk.Buffer
	geometryMem vk.DeviceMemory
	blasBuf     vk.Buffer
	blasMem     vk.DeviceMemory
	blas        vk.AccelerationStructure
	textures    []rtTextureData
}

// --- glTF loading ---

func rtLoadGLTFModel(dev vk.Device, gpu vk.PhysicalDevice, queue vk.Queue, cmdPool vk.CommandPool, path string) rtModelData {
	doc, err := gltf.Open(path)
	if err != nil {
		log.Fatal("gltf.Open:", err)
	}
	if len(doc.Scenes) == 0 {
		log.Fatal("gltf model has no scenes")
	}

	var prims []rtPrimitiveData
	activeScene := 0
	if doc.Scene != nil {
		activeScene = *doc.Scene
	}
	if activeScene < 0 || activeScene >= len(doc.Scenes) {
		log.Fatalf("gltf scene index %d out of range", activeScene)
	}

	textures := rtLoadGLTFTextures(dev, gpu, queue, cmdPool, doc, filepath.Dir(path))
	var visitNode func(nodeIndex int, parentTransform [16]float32)
	visitNode = func(nodeIndex int, parentTransform [16]float32) {
		node := doc.Nodes[nodeIndex]
		worldTransform := rtMultiplyMat4(parentTransform, rtGLTFNodeTransform(node))

		if node.Mesh != nil {
			meshIndex := *node.Mesh
			mesh := doc.Meshes[meshIndex]
			for pi, prim := range mesh.Primitives {
				positions, err := modeler.ReadPosition(doc, doc.Accessors[prim.Attributes[gltf.POSITION]], nil)
				if err != nil {
					log.Fatalf("Node %d mesh %d prim %d ReadPosition: %v", nodeIndex, meshIndex, pi, err)
				}
				normals, err := modeler.ReadNormal(doc, doc.Accessors[prim.Attributes[gltf.NORMAL]], nil)
				if err != nil {
					log.Fatalf("Node %d mesh %d prim %d ReadNormal: %v", nodeIndex, meshIndex, pi, err)
				}
				uvs, err := modeler.ReadTextureCoord(doc, doc.Accessors[prim.Attributes[gltf.TEXCOORD_0]], nil)
				if err != nil {
					log.Fatalf("Node %d mesh %d prim %d ReadTextureCoord: %v", nodeIndex, meshIndex, pi, err)
				}
				indices, err := modeler.ReadIndices(doc, doc.Accessors[*prim.Indices], nil)
				if err != nil {
					log.Fatalf("Node %d mesh %d prim %d ReadIndices: %v", nodeIndex, meshIndex, pi, err)
				}

				// Interleave: pos3 + normal3 + uv2 = 8 floats per vertex
				vertices := make([]float32, 0, len(positions)*8)
				for i := range positions {
					nx, ny, nz := rtTransformNormal(worldTransform, normals[i][0], normals[i][1], normals[i][2])
					vertices = append(vertices,
						positions[i][0], positions[i][1], positions[i][2],
						nx, ny, nz,
						uvs[i][0], uvs[i][1],
					)
				}

				log.Printf("  Node %d mesh %d prim %d: %d verts, %d indices", nodeIndex, meshIndex, pi, len(positions), len(indices))

				rtUsage := vk.BufferUsageFlags(vk.BufferUsageShaderDeviceAddressBit | vk.BufferUsageAccelerationStructureBuildInputReadOnlyBit | vk.BufferUsageStorageBufferBit)
				vertexBuf, vertexMem, err := ash.NewBufferWithDeviceAddress(dev, gpu, rtUsage,
					uint64(len(vertices)*4), unsafe.Pointer(&vertices[0]))
				if err != nil {
					log.Fatal(err)
				}
				indexBuf, indexMem, err := ash.NewBufferWithDeviceAddress(dev, gpu, rtUsage,
					uint64(len(indices)*4), unsafe.Pointer(&indices[0]))
				if err != nil {
					log.Fatal(err)
				}

				baseColorTex := int32(0)
				occlusionTex := int32(-1)
				if prim.Material != nil && *prim.Material >= 0 && *prim.Material < len(doc.Materials) {
					material := doc.Materials[*prim.Material]
					if material != nil {
						if material.PBRMetallicRoughness != nil && material.PBRMetallicRoughness.BaseColorTexture != nil {
							baseColorTex = int32(material.PBRMetallicRoughness.BaseColorTexture.Index + 1)
						}
						if material.OcclusionTexture != nil && material.OcclusionTexture.Index != nil {
							occlusionTex = int32(*material.OcclusionTexture.Index + 1)
						}
					}
				}

				prims = append(prims, rtPrimitiveData{
					vertexBuf: vertexBuf, vertexMem: vertexMem,
					indexBuf: indexBuf, indexMem: indexMem,
					vertexCount:   uint32(len(positions)),
					triangleCount: uint32(len(indices) / 3),
					transform:     rtVKTransformMatrix(worldTransform),
					baseColorTex:  baseColorTex,
					occlusionTex:  occlusionTex,
				})
			}
		}
		for _, childIndex := range node.Children {
			visitNode(childIndex, worldTransform)
		}
	}

	for _, rootNode := range doc.Scenes[activeScene].Nodes {
		visitNode(rootNode, rtIdentityMat4())
	}

	if len(prims) == 0 {
		log.Fatal("gltf model has no primitives")
	}

	blasBuf, blasMem, blas := rtBuildBLAS(dev, gpu, queue, cmdPool, prims)
	geometryBuf, geometryMem := rtCreateGeometryNodesBuffer(dev, gpu, prims)
	return rtModelData{
		primitives:  prims,
		geometryBuf: geometryBuf,
		geometryMem: geometryMem,
		blasBuf:     blasBuf,
		blasMem:     blasMem,
		blas:        blas,
		textures:    textures,
	}
}

// --- Texture loading ---

func rtDestroyTexture(dev vk.Device, texture rtTextureData) {
	vk.DestroySampler(dev, texture.sampler, nil)
	vk.DestroyImageView(dev, texture.view, nil)
	vk.DestroyImage(dev, texture.image, nil)
	vk.FreeMemory(dev, texture.memory, nil)
}

func rtCreateTexture(dev vk.Device, gpu vk.PhysicalDevice, queue vk.Queue, cmdPool vk.CommandPool, width, height uint32, pixels []byte, samplerInfo vk.SamplerCreateInfo) rtTextureData {
	stagingBuf, stagingMem, err := ash.NewBufferWithDeviceAddress(dev, gpu, vk.BufferUsageFlags(vk.BufferUsageTransferSrcBit), uint64(len(pixels)), unsafe.Pointer(&pixels[0]))
	if err != nil {
		log.Fatal(err)
	}

	var img vk.Image
	if err := vk.Error(vk.CreateImage(dev, &vk.ImageCreateInfo{
		SType:     vk.StructureTypeImageCreateInfo,
		ImageType: vk.ImageType2d,
		Format:    vk.FormatR8g8b8a8Unorm,
		Extent:    vk.Extent3D{Width: width, Height: height, Depth: 1},
		MipLevels: 1, ArrayLayers: 1, Samples: vk.SampleCount1Bit,
		Tiling: vk.ImageTilingOptimal,
		Usage:  vk.ImageUsageFlags(vk.ImageUsageTransferDstBit | vk.ImageUsageSampledBit),
	}, nil, &img)); err != nil {
		log.Fatal("CreateImage (texture):", err)
	}

	var memReqs vk.MemoryRequirements
	vk.GetImageMemoryRequirements(dev, img, &memReqs)
	memReqs.Deref()
	memIdx, _ := vk.FindMemoryTypeIndex(gpu, memReqs.MemoryTypeBits, vk.MemoryPropertyDeviceLocalBit)
	var mem vk.DeviceMemory
	if err := vk.Error(vk.AllocateMemory(dev, &vk.MemoryAllocateInfo{
		SType:           vk.StructureTypeMemoryAllocateInfo,
		AllocationSize:  memReqs.Size,
		MemoryTypeIndex: memIdx,
	}, nil, &mem)); err != nil {
		log.Fatal("AllocateMemory (texture):", err)
	}
	vk.BindImageMemory(dev, img, mem, 0)

	cmd := rtBeginOneTimeCmd(dev, cmdPool)
	rangeColor := vk.ImageSubresourceRange{AspectMask: vk.ImageAspectFlags(vk.ImageAspectColorBit), LevelCount: 1, LayerCount: 1}
	vk.CmdPipelineBarrier(cmd,
		vk.PipelineStageFlags(vk.PipelineStageTopOfPipeBit), vk.PipelineStageFlags(vk.PipelineStageTransferBit),
		0, 0, nil, 0, nil, 1, []vk.ImageMemoryBarrier{{
			SType:     vk.StructureTypeImageMemoryBarrier,
			OldLayout: vk.ImageLayoutUndefined, NewLayout: vk.ImageLayoutTransferDstOptimal,
			Image: img, SubresourceRange: rangeColor,
			DstAccessMask:       vk.AccessFlags(vk.AccessTransferWriteBit),
			SrcQueueFamilyIndex: vk.QueueFamilyIgnored, DstQueueFamilyIndex: vk.QueueFamilyIgnored,
		}})
	vk.CmdCopyBufferToImage(cmd, stagingBuf, img, vk.ImageLayoutTransferDstOptimal, 1, []vk.BufferImageCopy{{
		ImageSubresource: vk.ImageSubresourceLayers{AspectMask: vk.ImageAspectFlags(vk.ImageAspectColorBit), LayerCount: 1},
		ImageExtent:      vk.Extent3D{Width: width, Height: height, Depth: 1},
	}})
	vk.CmdPipelineBarrier(cmd,
		vk.PipelineStageFlags(vk.PipelineStageTransferBit), vk.PipelineStageFlags(vk.PipelineStageFragmentShaderBit|vk.PipelineStageRayTracingShaderBit),
		0, 0, nil, 0, nil, 1, []vk.ImageMemoryBarrier{{
			SType:     vk.StructureTypeImageMemoryBarrier,
			OldLayout: vk.ImageLayoutTransferDstOptimal, NewLayout: vk.ImageLayoutShaderReadOnlyOptimal,
			Image: img, SubresourceRange: rangeColor,
			SrcAccessMask: vk.AccessFlags(vk.AccessTransferWriteBit), DstAccessMask: vk.AccessFlags(vk.AccessShaderReadBit),
			SrcQueueFamilyIndex: vk.QueueFamilyIgnored, DstQueueFamilyIndex: vk.QueueFamilyIgnored,
		}})
	rtEndOneTimeCmd(dev, queue, cmdPool, cmd)

	vk.DestroyBuffer(dev, stagingBuf, nil)
	vk.FreeMemory(dev, stagingMem, nil)

	var view vk.ImageView
	if err := vk.Error(vk.CreateImageView(dev, &vk.ImageViewCreateInfo{
		SType:            vk.StructureTypeImageViewCreateInfo,
		Image:            img,
		ViewType:         vk.ImageViewType2d,
		Format:           vk.FormatR8g8b8a8Unorm,
		SubresourceRange: rangeColor,
	}, nil, &view)); err != nil {
		log.Fatal("CreateImageView (texture):", err)
	}

	var sampler vk.Sampler
	if err := vk.Error(vk.CreateSampler(dev, &samplerInfo, nil, &sampler)); err != nil {
		log.Fatal("CreateSampler (texture):", err)
	}

	return rtTextureData{image: img, memory: mem, view: view, sampler: sampler}
}

func rtDefaultSamplerCreateInfo() vk.SamplerCreateInfo {
	return vk.SamplerCreateInfo{
		SType:        vk.StructureTypeSamplerCreateInfo,
		MagFilter:    vk.FilterLinear,
		MinFilter:    vk.FilterLinear,
		MipmapMode:   vk.SamplerMipmapModeLinear,
		AddressModeU: vk.SamplerAddressModeRepeat,
		AddressModeV: vk.SamplerAddressModeRepeat,
		AddressModeW: vk.SamplerAddressModeRepeat,
		MaxLod:       0,
		BorderColor:  vk.BorderColorIntOpaqueWhite,
	}
}

func rtCreateDummyTexture(dev vk.Device, gpu vk.PhysicalDevice, queue vk.Queue, cmdPool vk.CommandPool) rtTextureData {
	return rtCreateTexture(dev, gpu, queue, cmdPool, 1, 1, []byte{255, 255, 255, 255}, rtDefaultSamplerCreateInfo())
}

func rtLoadGLTFTextures(dev vk.Device, gpu vk.PhysicalDevice, queue vk.Queue, cmdPool vk.CommandPool, doc *gltf.Document, baseDir string) []rtTextureData {
	textures := make([]rtTextureData, 0, len(doc.Textures)+1)
	textures = append(textures, rtCreateDummyTexture(dev, gpu, queue, cmdPool))
	for i, tex := range doc.Textures {
		if tex == nil || tex.Source == nil {
			log.Printf("Texture %d has no source, using fallback texture", i)
			textures = append(textures, rtCreateDummyTexture(dev, gpu, queue, cmdPool))
			continue
		}

		pixels, width, height, err := rtDecodeGLTFTexture(doc, baseDir, *tex.Source)
		if err != nil {
			log.Fatalf("decodeGLTFTexture %d: %v", i, err)
		}
		textures = append(textures, rtCreateTexture(dev, gpu, queue, cmdPool, width, height, pixels, rtSamplerCreateInfoForTexture(doc, tex)))
	}
	return textures
}

func rtDecodeGLTFTexture(doc *gltf.Document, baseDir string, imageIndex int) ([]byte, uint32, uint32, error) {
	if imageIndex < 0 || imageIndex >= len(doc.Images) {
		return nil, 0, 0, fmt.Errorf("image index %d out of range", imageIndex)
	}
	imageDef := doc.Images[imageIndex]
	if imageDef == nil {
		return nil, 0, 0, fmt.Errorf("image %d is nil", imageIndex)
	}

	var raw []byte
	var err error
	switch {
	case imageDef.IsEmbeddedResource():
		raw, err = imageDef.MarshalData()
	case imageDef.URI != "":
		raw, err = os.ReadFile(filepath.Join(baseDir, imageDef.URI))
	case imageDef.BufferView != nil:
		return nil, 0, 0, fmt.Errorf("bufferView-backed images are not supported")
	default:
		return nil, 0, 0, fmt.Errorf("image %d has no data source", imageIndex)
	}
	if err != nil {
		return nil, 0, 0, err
	}

	decoded, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return nil, 0, 0, err
	}
	bounds := decoded.Bounds()
	rgba := image.NewRGBA(bounds)
	draw.Draw(rgba, bounds, decoded, bounds.Min, draw.Src)
	return rgba.Pix, uint32(bounds.Dx()), uint32(bounds.Dy()), nil
}

func rtSamplerCreateInfoForTexture(doc *gltf.Document, tex *gltf.Texture) vk.SamplerCreateInfo {
	info := rtDefaultSamplerCreateInfo()
	if tex == nil || tex.Sampler == nil || *tex.Sampler < 0 || *tex.Sampler >= len(doc.Samplers) {
		return info
	}
	sampler := doc.Samplers[*tex.Sampler]
	if sampler == nil {
		return info
	}

	info.MagFilter = rtMagFilterFromGLTF(sampler.MagFilter)
	info.MinFilter, info.MipmapMode = rtMinFilterFromGLTF(sampler.MinFilter)
	info.AddressModeU = rtAddressModeFromGLTF(sampler.WrapS)
	info.AddressModeV = rtAddressModeFromGLTF(sampler.WrapT)
	return info
}

func rtMagFilterFromGLTF(filter gltf.MagFilter) vk.Filter {
	switch filter {
	case gltf.MagNearest:
		return vk.FilterNearest
	default:
		return vk.FilterLinear
	}
}

func rtMinFilterFromGLTF(filter gltf.MinFilter) (vk.Filter, vk.SamplerMipmapMode) {
	switch filter {
	case gltf.MinNearest, gltf.MinNearestMipMapNearest, gltf.MinNearestMipMapLinear:
		return vk.FilterNearest, vk.SamplerMipmapModeNearest
	case gltf.MinLinearMipMapNearest:
		return vk.FilterLinear, vk.SamplerMipmapModeNearest
	default:
		return vk.FilterLinear, vk.SamplerMipmapModeLinear
	}
}

func rtAddressModeFromGLTF(mode gltf.WrappingMode) vk.SamplerAddressMode {
	switch mode {
	case gltf.WrapClampToEdge:
		return vk.SamplerAddressModeClampToEdge
	case gltf.WrapMirroredRepeat:
		return vk.SamplerAddressModeMirroredRepeat
	default:
		return vk.SamplerAddressModeRepeat
	}
}

// --- Matrix helpers ---

func rtIdentityMat4() [16]float32 {
	return [16]float32{
		1, 0, 0, 0,
		0, 1, 0, 0,
		0, 0, 1, 0,
		0, 0, 0, 1,
	}
}

func rtMultiplyMat4(a, b [16]float32) [16]float32 {
	var out [16]float32
	for col := 0; col < 4; col++ {
		for row := 0; row < 4; row++ {
			var sum float32
			for k := 0; k < 4; k++ {
				sum += a[k*4+row] * b[col*4+k]
			}
			out[col*4+row] = sum
		}
	}
	return out
}

func rtGLTFNodeTransform(node *gltf.Node) [16]float32 {
	if node.Matrix != gltf.DefaultMatrix && node.Matrix != [16]float64{} {
		return rtGLTFMatrixToArray(node.Matrix)
	}

	translation := node.TranslationOrDefault()
	rotation := node.RotationOrDefault()
	scale := node.ScaleOrDefault()

	var t ash.Mat4x4
	t.Translate(float32(translation[0]), float32(translation[1]), float32(translation[2]))

	var r ash.Mat4x4
	r.FromQuat(&ash.Quat{
		float32(rotation[0]),
		float32(rotation[1]),
		float32(rotation[2]),
		float32(rotation[3]),
	})

	var rs ash.Mat4x4
	rs.ScaleAniso(&r, float32(scale[0]), float32(scale[1]), float32(scale[2]))

	var trs ash.Mat4x4
	trs.Mult(&t, &rs)
	return rtMat4ToArray(&trs)
}

func rtTransformNormal(m [16]float32, nx, ny, nz float32) (float32, float32, float32) {
	ox := m[0]*nx + m[4]*ny + m[8]*nz
	oy := m[1]*nx + m[5]*ny + m[9]*nz
	oz := m[2]*nx + m[6]*ny + m[10]*nz
	l := float32(math.Sqrt(float64(ox*ox + oy*oy + oz*oz)))
	if l > 0 {
		ox /= l
		oy /= l
		oz /= l
	}
	return ox, oy, oz
}

func rtVKTransformMatrix(m [16]float32) [12]float32 {
	return [12]float32{
		m[0], m[4], m[8], m[12],
		m[1], m[5], m[9], m[13],
		m[2], m[6], m[10], m[14],
	}
}

func rtGLTFMatrixToArray(m [16]float64) [16]float32 {
	var out [16]float32
	for i := range out {
		out[i] = float32(m[i])
	}
	return out
}

func rtMat4ToArray(m *ash.Mat4x4) [16]float32 {
	var out [16]float32
	for col := 0; col < 4; col++ {
		for row := 0; row < 4; row++ {
			out[col*4+row] = m[col][row]
		}
	}
	return out
}
