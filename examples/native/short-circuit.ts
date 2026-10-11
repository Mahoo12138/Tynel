function mark(id: number, value: boolean): boolean {
  console.log(id);
  return value;
}

function both(left: boolean): boolean {
  return mark(1, left) && mark(2, true);
}

function either(left: boolean): boolean {
  return mark(3, left) || mark(4, false);
}

console.log(both(false));
console.log(both(true));
console.log(either(true));
console.log(either(false));
