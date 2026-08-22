declare var GPUBufferUsage: {
	readonly MAP_READ: GPUFlagsConstant;
	readonly MAP_WRITE: GPUFlagsConstant;
	readonly COPY_SRC: GPUFlagsConstant;
	readonly COPY_DST: GPUFlagsConstant;
	readonly INDEX: GPUFlagsConstant;
	readonly VERTEX: GPUFlagsConstant;
	readonly UNIFORM: GPUFlagsConstant;
	readonly STORAGE: GPUFlagsConstant;
	readonly INDIRECT: GPUFlagsConstant;
	readonly QUERY_RESOLVE: GPUFlagsConstant;
};

declare var GPUTextureUsage: {
	readonly COPY_SRC: GPUFlagsConstant;
	readonly COPY_DST: GPUFlagsConstant;
	readonly TEXTURE_BINDING: GPUFlagsConstant;
	readonly STORAGE_BINDING: GPUFlagsConstant;
	readonly RENDER_ATTACHMENT: GPUFlagsConstant;
};

declare var GPUShaderStage: {
	readonly VERTEX: GPUFlagsConstant;
	readonly FRAGMENT: GPUFlagsConstant;
	readonly COMPUTE: GPUFlagsConstant;
};
