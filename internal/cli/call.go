package cli

import (
	"errors"
	"fmt"
	"reflect"
)

// call разворачивает ответ сгенерированного клиента в разобранный JSON.
//
// У каждой из 161 операции свой тип ответа, но устроены они одинаково: поле
// Body []byte и метод StatusCode(). Общего интерфейса генератор не даёт, а
// писать разбор на каждом вызове — значит однажды написать его не так:
// при сетевой ошибке клиент возвращает (nil, err), и обращение к
// resp.StatusCode() роняет CLI паникой вместо внятного сообщения.
//
// Вызывается прямо на результате метода клиента:
//
//	v, err := call(c.VmsGetVmWithResponse(ctx, project, vm))
func call(resp any, err error) (any, error) {
	status, body, err := unwrap(resp, err)
	if err != nil {
		return nil, explainTransport(err)
	}
	return decode(status, body, nil)
}

// callOK — то же для команд, которым тело ответа не нужно.
func callOK(resp any, err error) error {
	_, err = call(resp, err)
	return err
}

func unwrap(resp any, err error) (int, []byte, error) {
	if err != nil {
		return 0, nil, err
	}
	rv := reflect.ValueOf(resp)
	if !rv.IsValid() || (rv.Kind() == reflect.Pointer && rv.IsNil()) {
		return 0, nil, errors.New("клиент вернул пустой ответ без ошибки")
	}
	method := rv.MethodByName("StatusCode")
	if !method.IsValid() {
		return 0, nil, fmt.Errorf("у ответа %T нет StatusCode()", resp)
	}
	out := method.Call(nil)
	if len(out) != 1 || out[0].Kind() != reflect.Int {
		return 0, nil, fmt.Errorf("StatusCode() у %T вернул неожиданное значение", resp)
	}
	status := int(out[0].Int())

	field := reflect.Indirect(rv).FieldByName("Body")
	if !field.IsValid() {
		return 0, nil, fmt.Errorf("у ответа %T нет поля Body", resp)
	}
	body, ok := field.Interface().([]byte)
	if !ok {
		return 0, nil, fmt.Errorf("поле Body у %T не []byte", resp)
	}
	return status, body, nil
}
