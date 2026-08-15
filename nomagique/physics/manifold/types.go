package manifold

type Grid struct {
	X uint32 `json:"x"`
	Y uint32 `json:"y"`
	Z uint32 `json:"z"`
}

type Rho struct {
	Mass     float64 `json:"mass"`
	Peak     float64 `json:"peak"`
	Entropy  float64 `json:"entropy"`
	Gradient float64 `json:"gradient"`
	CenterX  float64 `json:"centerX"`
	CenterZ  float64 `json:"centerZ"`
	SpreadX  float64 `json:"spreadX"`
	SpreadZ  float64 `json:"spreadZ"`
}

type Oscillators struct {
	Coherence float64 `json:"coherence"`
	Kinetic   float64 `json:"kinetic"`
	Thermal   float64 `json:"thermal"`
	Omega     float64 `json:"omega"`
}

type Clamp struct {
	Lane      int     `json:"lane"`
	PositionX float64 `json:"positionX"`
	PositionZ float64 `json:"positionZ"`
	Rho       float64 `json:"rho"`
	MomentumX float64 `json:"momentumX"`
	MomentumY float64 `json:"momentumY"`
	MomentumZ float64 `json:"momentumZ"`
	Energy    float64 `json:"energy"`
	Pressure  float64 `json:"pressure"`
}

type Carrier struct {
	Role     string  `json:"role"`
	CellX    float64 `json:"cell_x"`
	CellZ    float64 `json:"cell_z"`
	Strength float64 `json:"strength"`
}

type Particle struct {
	Role      string  `json:"role"`
	CellX     float64 `json:"cell_x"`
	CellY     float64 `json:"cell_y"`
	CellZ     float64 `json:"cell_z"`
	Phase     float64 `json:"phase"`
	Omega     float64 `json:"omega"`
	Amplitude float64 `json:"amplitude"`
	Heat      float64 `json:"heat"`
	VelX      float64 `json:"vel_x"`
	VelY      float64 `json:"vel_y"`
	VelZ      float64 `json:"vel_z"`
	Speed     float64 `json:"speed"`
}
