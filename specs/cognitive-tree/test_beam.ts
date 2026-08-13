import { CognitiveModel } from './src/lib/CognitiveModel';

const initialData = {
  "Truck": ["blue_cab_big_wheel", "blue_cab_flat_bed", "white_cab_big_wheel", "red_cab_flat_bed", "heavy_duty_truck", "diesel_engine_roar"],
  "Car": ["blue_hood_small_tire", "red_hood_small_tire", "blue_hood_spoiler", "white_hood_small_tire", "fast_sports_car", "electric_sedan"],
  "Bike": ["red_tank_two_wheel", "black_tank_two_wheel", "blue_tank_two_wheel", "mountain_bike_tires", "carbon_fiber_frame"]
};

const m = new CognitiveModel();
for (const [label, sequences] of Object.entries(initialData)) {
  sequences.forEach(seq => m.train(seq, label));
}

console.log(m.beamSearch("blue_", 3, 5));
